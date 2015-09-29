package console

import (
	"flag"
	"fmt"

	"../presenter"
	"../repo"
	"../usecase"
	"../viewmodel"

	"github.com/bndr/gotabulate"
)

var cmdListProjects = flag.Bool("cmd.list", false, "List all projects and their status")
var cmdShowProject = flag.String("cmd.project", "", "Show a specific project by name")

func StartViewer(projectRepo *repo.Projects, buildRepo repo.BuildResultRepo, buildOutputReader usecase.BuildOutputReader) {
	if *cmdListProjects {
		if err := projectOverviewListing(projectRepo, buildRepo); err != nil {
			fmt.Printf("Failed to list the projects: %s\n", err)
			return
		}
	}

	if *cmdShowProject != "" {
		if err := projectShowDetail(*cmdShowProject, projectRepo, buildRepo); err != nil {
			fmt.Printf("Failed to show details of project %s: %s\n", *cmdShowProject, err)
			return
		}
	}

	return

}

func projectShowDetail(projectName string, projectRepo *repo.Projects, buildRepo repo.BuildResultRepo) (err error) {
	builds, err := presenter.CreateBuildResultListing(projectName, buildRepo)
	if err != nil {
		return fmt.Errorf("Failed to create build results listing %s: %s", projectName, err)
	}

	project := projectRepo.FindProject(projectName)
	if project == nil {
		return fmt.Errorf("Project %s not found", projectName)
	}

	// holds all the builds per commit
	var buildsByCommit []*CommitBuilds

	// finding all the builds which belong to a commit
	for _, b := range builds {
		if idx := findBuildIndex(buildsByCommit, b.Commit); idx >= 0 {
			buildsByCommit[idx].Builds = append(buildsByCommit[idx].Builds, b)
			continue
		}
		buildsByCommit = append(buildsByCommit, &CommitBuilds{Commit: b.Commit, Builds: []*viewmodel.BuildInfo{b}})
	}

	// creating the header row
	var headers []string
	headers = []string{""}
	for _, builder := range project.GetBuilder() {
		headers = append(headers, builder.Name)
		for i := 0; i < len(builder.Slave)-1; i++ {
			headers = append(headers, "")
		}
	}

	// columnBuilderSlave is needed to keep track of how many rows we have
	// and what key we look for. Key consists of builder+slave. (so we can look up later if we have any builds for that cell)
	var columnBuilderSlave []string
	var rows [][]string

	// Create cells Columns Table
	var row []string
	row = []string{""}
	for _, builder := range project.GetBuilder() {
		for _, slave := range builder.Slave {
			columnBuilderSlave = append(columnBuilderSlave, builder.Name+slave)
			row = append(row, slave)
		}
	}
	rows = append(rows, row)

	// Adding an empty seperator row:
	rows = append(rows, []string{})

	for _, buildinfo := range buildsByCommit {
		row = []string{buildinfo.Builds[0].ShortCommit}
		for i := 0; i < len(columnBuilderSlave); i++ {

			if found := FindBuildByKey(columnBuilderSlave[i], buildinfo.Builds); found != nil {
				row = append(row, found.Status)
			}

		}
		rows = append(rows, row)
	}
	tabulate := gotabulate.Create(rows)
	tabulate.SetAlign("center")
	// Set Headers
	tabulate.SetHeaders(headers)

	// Render
	fmt.Println(tabulate.Render("grid"))
	return
}

type CommitBuilds struct {
	Commit string
	Builds []*viewmodel.BuildInfo
}

func FindBuildByKey(key string, builds []*viewmodel.BuildInfo) *viewmodel.BuildInfo {
	for _, build := range builds {
		if key == build.Builder+build.Slave {
			return build
		}
	}
	return nil
}

func findBuildIndex(buildInfos []*CommitBuilds, commit string) int {
	for i, info := range buildInfos {
		if info.Commit == commit {
			return i
		}
	}
	return -1
}

func projectOverviewListing(projectRepo *repo.Projects, buildRepo repo.BuildResultRepo) error {
	projects, err := presenter.CreateProjectListing(projectRepo, buildRepo)
	if err != nil {
		return err
	}

	var rows [][]string
	for _, proj := range projects {
		rows = append(rows, []string{proj.Name, proj.Timestamp.String(), proj.Status})
	}

	tabulate := gotabulate.Create(rows)
	tabulate.SetAlign("center")

	// Set Headers
	tabulate.SetHeaders([]string{"Project", "Last build time", "Status"})

	// Render
	fmt.Println(tabulate.Render("grid"))
	return nil
}
