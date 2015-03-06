// Master: https://megrim.uk/app.html?token=1425326953d6e0182006184937838bf8ca7b82de31
package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"

	context "golang.org/x/net/context"
	"google.golang.org/grpc"

	"./exec"
	pbhooks "./hooks/proto"
	pb "./proto"
)

var listenerAddr = flag.String("listener", "localhost:50052", "Notify server listening address")

func main() {
	flag.Parse()

	launchNotifyServer(*listenerAddr)

	return
}

func executeCommand(dir string, args ...string) {
	conn, err := grpc.Dial("localhost:50051")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	var buf bytes.Buffer
	c := pb.NewBuilderClient(conn)

	cmd := new(exec.RemoteCmd)
	cmd.Dir = dir
	cmd.Args = args
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	cmd.Run(c)
	fmt.Println(buf.String())
	return
}

func launchNotifyServer(address string) {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		panic(err)
	}
	s := grpc.NewServer()
	pbhooks.RegisterChangeSourceServer(s, &server{})
	s.Serve(lis)
}

type server struct{}

func (self *server) Notify(ctx context.Context, req *pbhooks.ChangeRequest) (*pbhooks.ChangeResponse, error) {
	fmt.Println(req)
	// executeCommand(req.Dir, "git", "status")
	return &pbhooks.ChangeResponse{}, nil
}
