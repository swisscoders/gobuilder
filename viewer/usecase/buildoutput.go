package usecase

import (
	"fmt"
	"io/ioutil"
	"path"
)

type BuildOutputReader interface {
	GetOutputStreamContent(project, builder, commit, slave string) (stdout []byte, stderr []byte, err error)
}

func NewBuildOutputReader(buildLogsPath string) BuildOutputReader {
	return &buildOutput{path: buildLogsPath}
}

type buildOutput struct {
	path string
}

func (self *buildOutput) GetOutputStreamContent(project, builder, commit, slave string) (stdout []byte, stderr []byte, err error) {
	filePath := path.Join(self.path, project, builder, fmt.Sprintf("%s.%s", commit, slave))

	stdoutContent, errStdout := ioutil.ReadFile(filePath + ".stdout.log")
	stderrContent, errStderr := ioutil.ReadFile(filePath + ".stderr.log")

	var errs []error
	if errStdout != nil {
		stdoutContent = []byte("Failed to read: " + errStdout.Error())
		errs = append(errs, errStdout)
	}

	if errStderr != nil {
		stderrContent = []byte("Failed to read: " + errStderr.Error())
		errs = append(errs, errStderr)
	}

	if len(errs) > 0 {
		err = fmt.Errorf("Failed to read output stream content: %v", errs)
	}

	return stdoutContent, stderrContent, err
}
