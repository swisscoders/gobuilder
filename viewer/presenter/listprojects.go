package presenter

import (
	"fmt"

	pb "../../proto"
	"../repo"
	"../viewmodel"
)

func CreateProjectListing(projectRepo *repo.Projects, buildRepo repo.BuildResultRepo) (projects []*viewmodel.Project, err error) {
	for _, proj := range projectRepo.Projects() {
		var project viewmodel.Project

		keys, results, err := buildRepo.Results(proj.Name)
		if err != nil {
			return nil, fmt.Errorf("Project %s error while fetching build results: %s", proj.Name, err)
		}
		project.Name = proj.Name
		project.Status = "Success"

		// Results of the latest commit
		var lastResults []*pb.BuildResult
		var lastCommit string

		if results != nil && len(results) > 0 {
			lastCommit = results[0].ChangeRequest.Commit
			for _, res := range results {
				if lastCommit == res.ChangeRequest.Commit {
					lastResults = append(lastResults, res)
					continue
				}
				break
			}
			if !repo.IsBuildStatusOK(lastResults...) {
				project.Status = "Failed"
			}
		} else {
			project.Status = "Not built yet"
		}

		project.Timestamp = keys[0].Timestamp()

		projects = append(projects, &project)
	}
	return
}
