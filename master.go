package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"syscall"

	pb "./proto"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

func main() {
	flag.Parse()
	go startServer()

	conn, err := grpc.Dial("localhost:50051")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	c := pb.NewBuilderClient(conn)
	r, err := c.Execute(context.Background(), &pb.ExecutionRequest{Args: []string{"ls", "-lhsa"}})
	fmt.Println(r, err)

	for {
		resp, err := r.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			panic(err)
		}
		fmt.Println(string(resp.Stdout), resp.GetStatus())
	}

	return
}

type server struct{}

func (self *server) Execute(req *pb.ExecutionRequest, resp pb.Builder_ExecuteServer) error {
	glog.Infof("Request to execute %v", req.Args)

	cmd := exec.Command(req.Args[0], req.Args[1:]...)
	cmd.Env = convertEnv(req.GetEnv())
	cmd.Stdin = bytes.NewReader(req.Stdin)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	streamReader(stdoutPipe, stderrPipe, resp)
	err = cmd.Run()

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
	s := &pb.ExecutionResponse_Status{
		CoreDump:   status.CoreDump(),
		Exited:     status.Exited(),
		ExitStatus: int32(status.ExitStatus()),
		Signaled:   status.Signaled(),
		Signal:     int32(status.Signal()),
	}
	resp.Send(&pb.ExecutionResponse{Status: s})
	return nil
}

func streamReader(stdout io.ReadCloser, stderr io.ReadCloser, resp pb.Builder_ExecuteServer) {
	readerFn := func(in io.Reader) <-chan []byte {
		out := make(chan []byte)
		go func() {
			for {
				output := make([]byte, 4096)
				_, err := in.Read(output)
				if err != nil && err != io.EOF {
					// TODO(rn): Hand error to caller
					glog.Errorf("Failed to read input: %s", err)
					break
				}
				out <- output
				if err == io.EOF {
					close(out)
					break
				}
			}
		}()
		return out
	}
	go func() {
		stderrin := readerFn(stderr)
		stdoutin := readerFn(stdout)

		var stderrOnce sync.Once
		var stdoutOnce sync.Once

		var wg sync.WaitGroup
		wg.Add(2)
		stop := make(chan struct{})
		go func() {
			wg.Wait()
			stop <- struct{}{}
		}()
		for {
			select {
			case d, ok := <-stderrin:
				//fmt.Println("stderr", ok)
				if !ok {
					stderrOnce.Do(wg.Done)
					continue
				}
				resp.Send(&pb.ExecutionResponse{Stderr: d})
			case d, ok := <-stdoutin:
				//fmt.Println("stdout", ok)
				if !ok {
					stdoutOnce.Do(wg.Done)
					continue
				}
				resp.Send(&pb.ExecutionResponse{Stdout: d})
			case <-stop:
				break
			}
		}
	}()
}

func convertEnv(env []*pb.ExecutionRequest_Env) (newEnv []string) {
	for _, e := range env {
		newEnv = append(newEnv, fmt.Sprintf("%s=%s", e.Key, e.Value))
	}
	return
}

func startServer() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		panic(err)
	}
	s := grpc.NewServer()
	pb.RegisterBuilderServer(s, &server{})
	s.Serve(lis)
}
