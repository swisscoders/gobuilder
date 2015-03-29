// Slave
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"

	pb "./proto"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var address = flag.String("address", ":50051", "address of slave")

type signature struct {
	hasher hash.Hash
}

func newSignature(key []byte) *signature {
	return &signature{hasher: hmac.New(sha256.New, key)}
}

func (self *signature) VerifyEnv(env *pb.BuildEnv) error {
	if env.Sig == nil || len(env.Sig) == 0 {
		return fmt.Errorf("No signature present")
	}

	// We need to unset the signature before verify signature (feedback loop)
	sig := env.Sig
	env.Sig = nil
	// Now recreate signature
	err := self.SignEnv(env)
	// Restore old signature and assign freshly generated one
	env.Sig, sig = sig, env.Sig
	if err != nil {
	}
	if !hmac.Equal(sig, env.Sig) {
		return fmt.Errorf("Signature does not match")
	}
	return nil
}

func (self *signature) SignEnv(env *pb.BuildEnv) error {
	if env.Sig != nil {
		return fmt.Errorf("Signature already present")
	}

	b, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("While marshaling env proto: %s", err)
	}
	env.Sig = self.CreateMAC(b)
	return nil
}

func (self *signature) CreateMAC(message []byte) []byte {
	defer self.hasher.Reset()
	self.hasher.Write(message)
	return self.hasher.Sum(nil)
}

var sig *signature

func init() {
	keySize := 256 / 8
	key := make([]byte, keySize)
	i, err := rand.Read(key) // This is "crypto/rand"! Important!
	if err != nil {
		panic(err)
	}
	if i != keySize {
		panic(fmt.Sprintf("Requestes keysize not matched: expected %d, got %d", keySize, i))
	}
	sig = newSignature(key)
}

func main() {
	flag.Parse()

	// TODO(rn): Setup TLS
	lis, err := net.Listen("tcp", *address)
	if err != nil {
		panic(err)
	}
	s := grpc.NewServer()
	pb.RegisterBuilderServer(s, &server{})
	s.Serve(lis)
}

type server struct{}

func (self *server) Execute(req *pb.ExecutionRequest, resp pb.Builder_ExecuteServer) error {
	if len(req.Args) == 0 {
		return fmt.Errorf("Request has no command to execute.")
	}

	var args []string
	if len(req.Args) > 1 {
		args = req.Args[1:]
	}

	if req.BuildEnv == nil {
		return fmt.Errorf("No build environment present")
	} else if err := sig.VerifyEnv(req.BuildEnv); err != nil {
		return fmt.Errorf("Failure verifying build environment: %s", err)
	}
	cmd := exec.Command(req.Args[0], args...)
	cmd.Env = convertEnv(req.GetEnv())
	cmd.Stdin = bytes.NewReader(req.Stdin)
	cmd.Dir = req.BuildEnv.Path

	glog.V(1).Infof("Commands to execute %v (build dir: %s)", req, cmd.Dir)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err = cmd.Start(); err != nil {
		return err
	}
	streamReader(stdoutPipe, stderrPipe, resp)
	err = cmd.Wait()

	var status syscall.WaitStatus
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			status = err.(*exec.ExitError).Sys().(syscall.WaitStatus)
		} else {
			return err
		}
	} else if cmd.ProcessState != nil {
		status = cmd.ProcessState.Sys().(syscall.WaitStatus)
	}
	s := &pb.Status{
		CoreDump:   status.CoreDump(),
		Exited:     status.Exited(),
		ExitStatus: int32(status.ExitStatus()),
		Signaled:   status.Signaled(),
		Signal:     int32(status.Signal()),
	}
	resp.Send(&pb.ExecutionResponse{Status: s})
	return nil
}

func (self *server) Setup(ctx context.Context, req *pb.SetupRequest) (resp *pb.SetupResponse, err error) {
	var env pb.BuildEnv
	env.Path, err = ioutil.TempDir("", "build")
	if err != nil {
		return nil, fmt.Errorf("Cannot create temporary build directory: %s", err)
	}
	if err = sig.SignEnv(&env); err != nil {
		return nil, fmt.Errorf("Could not sign build environment: %s", err)
	}
	return &pb.SetupResponse{Env: &env}, nil
}

func (self *server) Teardown(ctx context.Context, req *pb.TeardownRequest) (resp *pb.TeardownResponse, err error) {
	if req.Env == nil {
		return nil, fmt.Errorf("No build environment present")
	} else if err := sig.VerifyEnv(req.Env); err != nil {
		return nil, fmt.Errorf("Failure verifying build environment: %s", err)
	}
	if err := os.RemoveAll(req.Env.Path); err != nil {
		return nil, fmt.Errorf("Could not tear down build environment: %s", err)
	}
	return &pb.TeardownResponse{}, nil
}

func streamReader(stdout io.ReadCloser, stderr io.ReadCloser, resp pb.Builder_ExecuteServer) {
	var wg sync.WaitGroup
	readerFn := func(wg *sync.WaitGroup, in io.Reader) <-chan []byte {
		out := make(chan []byte)
		go func() {
			for {
				output := make([]byte, 4096)
				n, err := in.Read(output)
				if err != nil && err != io.EOF {
					// TODO(rn): Hand error to caller
					glog.Errorf("Failed to read input: %s", err)
					break
				}
				out <- output[:n]
				if err == io.EOF {
					wg.Done()
					break
				}
			}
		}()
		return out
	}
	wg.Add(2)
	stderrin := readerFn(&wg, stderr)
	stdoutin := readerFn(&wg, stdout)

	stop := make(chan struct{})
	go func() {
		wg.Wait()
		stop <- struct{}{}
	}()
	for {
		select {
		case d := <-stderrin:
			resp.Send(&pb.ExecutionResponse{Stderr: d})
		case d := <-stdoutin:
			resp.Send(&pb.ExecutionResponse{Stdout: d})
		case <-stop:
			return
		}
	}
}

func convertEnv(env []*pb.ExecutionRequest_Env) (newEnv []string) {
	for _, e := range env {
		newEnv = append(newEnv, fmt.Sprintf("%s=%s", e.Key, e.Value))
	}
	return
}
