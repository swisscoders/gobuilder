package main

import (
	"bytes"
	"flag"
	"fmt"

	"google.golang.org/grpc"

	"./exec"
	pb "./proto"
)

func main() {
	flag.Parse()

	conn, err := grpc.Dial("localhost:50051")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	var buf bytes.Buffer
	c := pb.NewBuilderClient(conn)

	cmd := new(exec.RemoteCmd)
	cmd.Args = []string{"ls", "-lhsa"}
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	cmd.Run(c)
	fmt.Println(buf.String())

	return
}
