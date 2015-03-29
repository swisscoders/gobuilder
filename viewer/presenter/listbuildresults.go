package presenter

import (
	"fmt"
	"time"

	"../repo"
	"../viewmodel"
)

func CreateBuildResultListing(projectName string, buildRepo repo.BuildResultRepo) (builds []*viewmodel.BuildInfo, err error) {
	keys, results, err := buildRepo.Results(projectName)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch build results for project %s: %s", projectName, err)
	}

	for i, result := range results {
		var b viewmodel.BuildInfo
		b.Project = result.ChangeRequest.Project
		b.ShortCommit = result.ChangeRequest.Commit[0:18]
		b.Commit = result.ChangeRequest.Commit
		b.Builder = keys[i].Builder()
		b.Slave = keys[i].Slave()
		b.Status = "OK"
		if !repo.IsBuildStatusOK(result) {
			b.StatusClass = "fail" // css
			b.Status = "FAIL"
		}
		b.Repo = result.ChangeRequest.Repo
		b.Branch = result.ChangeRequest.Branch
		b.Files = result.ChangeRequest.Files

		b.Duration = time.Duration(result.Duration)
		b.Timestamp = keys[i].Timestamp()
		builds = append([]*viewmodel.BuildInfo{&b}, builds...)
	}
	return
}
