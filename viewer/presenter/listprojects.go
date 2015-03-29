package presenter

import (
	"fmt"

	"../repo"
	"../viewmodel"
)

func CreateProjectListing(projectRepo *repo.Projects, buildRepo repo.BuildResultRepo) (projects []*viewmodel.Project, err error) {
	for _, proj := range projectRepo.Projects() {
		var project viewmodel.Project

		keys, result, err := buildRepo.Results(proj.GetName())
		if err != nil {
			return nil, fmt.Errorf("Project %s error while fetching build results: %s", proj.GetName(), err)
		}

		project.Name = proj.GetName()
		project.Status = "Success"
		if !repo.IsBuildStatusOK(result...) {
			project.Status = "Failed"
		}

		project.Timestamp = keys[len(keys)-1].Timestamp()

		projects = append(projects, &project)
	}
	return
}
