package exec

import (
	"io"
	"io/ioutil"
	"os"
	"strings"

	pb "../proto"
	//pbhooks "../hooks/proto"
	"golang.org/x/net/context"
)

type RemoteCmd struct {
	//*pb.ExecutionRequest
	Args []string
	Env  []string

	BuildEnv *pb.BuildEnv

	Status *pb.Status

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	closeAfterExecute []io.Closer
}

func (self *RemoteCmd) StdinPipe() (io.WriteCloser, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	self.Stdin = r
	return w, nil
}

func (self *RemoteCmd) StdoutPipe() (io.Reader, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	self.Stdout = w
	self.closeAfterExecute = append(self.closeAfterExecute, w)
	return r, nil
}

func (self *RemoteCmd) StderrPipe() (io.Reader, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	self.Stderr = w
	self.closeAfterExecute = append(self.closeAfterExecute, w)
	return r, nil
}

func (self *RemoteCmd) setupFds() error {
	if self.Stdin == nil {
		f, err := os.Open(os.DevNull)
		if err != nil {
			return err
		}
		self.Stdin = f
		self.closeAfterExecute = append(self.closeAfterExecute, f)
	}

	if self.Stdout == nil {
		f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		self.Stdout = f
		self.closeAfterExecute = append(self.closeAfterExecute, f)
	}

	if self.Stderr == nil {
		f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		self.Stderr = f
		self.closeAfterExecute = append(self.closeAfterExecute, f)
	}
	return nil
}

func (self *RemoteCmd) closeAll(fds []io.Closer) {
	for _, fd := range fds {
		fd.Close()
	}
}

func (self *RemoteCmd) Setup(c pb.BuilderClient) (*pb.BuildEnv, error) {
	resp, err := c.Setup(context.Background(), &pb.SetupRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Env, nil
}

func (self *RemoteCmd) Teardown(c pb.BuilderClient, env *pb.BuildEnv) error {
	_, err := c.Teardown(context.Background(), &pb.TeardownRequest{Env: env})
	return err
}

func (self *RemoteCmd) Run(c pb.BuilderClient) error {
	var req = &pb.ExecutionRequest{
		Args:     self.Args,
		BuildEnv: self.BuildEnv,
	}

	self.setupFds()
	defer self.closeAll(self.closeAfterExecute)

	// TODO(rn): Support streaming stdin
	if self.Stdin != nil {
		req.Stdin, _ = ioutil.ReadAll(self.Stdin)
	}

	for _, kv := range self.Env {
		pair := strings.SplitN(kv, "=", 1)
		req.Env = append(req.Env, &pb.ExecutionRequest_Env{Key: pair[0], Value: pair[1]})
	}

	r, err := c.Execute(context.Background(), req)
	if err != nil {
		return err
	}

	for {
		resp, err := r.Recv()
		if err != nil {
			return err
		}

		if resp.Status != nil {
			self.Status = resp.Status
		}

		if resp.Stdout != nil {
			if _, err := self.Stdout.Write(resp.Stdout); err != nil {
				return err
			}
		}

		if resp.Stderr != nil {
			if _, err := self.Stderr.Write(resp.Stderr); err != nil {
				return err
			}
		}

		if err == io.EOF {
			return nil
		}

	}
	return nil
}
