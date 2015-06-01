package web

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"

	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"

	// "../presenter"
	// "../viewmodel"

	"../presenter"
	"../repo"
	"../usecase"
	"../viewmodel"

	tmpl "./template"
)

func StartViewer(projectRepo *repo.Projects, buildRepo repo.BuildResultRepo, buildOutputReader usecase.BuildOutputReader) {
	viewer := &webViewer{
		projectRepo:       projectRepo,
		buildRepo:         buildRepo,
		buildOutputReader: buildOutputReader,
	}

	goji.Get("/assets/*", http.StripPrefix("/assets", http.FileServer(http.Dir("web/assets"))))

	goji.Get("/", viewer.listProjects)
	goji.Get("/project/:id", viewer.showProject)
	goji.Get("/project/output/:project/:builder/:commit/:slave", viewer.showBuildOutput)

	goji.Serve()
	return
}

type webViewer struct {
	projectRepo       *repo.Projects
	buildRepo         repo.BuildResultRepo
	buildOutputReader usecase.BuildOutputReader
}

func (self *webViewer) listProjects(c web.C, w http.ResponseWriter, r *http.Request) {
	v := NewView()
	projects, err := presenter.CreateProjectListing(self.projectRepo, self.buildRepo)
	if err != nil {
		v.Feedback.Error("Error", err.Error())
	}
	v.Render(w, "project/index", projects)
}

func (self *webViewer) showBuildOutput(c web.C, w http.ResponseWriter, r *http.Request) {
	v := NewView()

	project := c.URLParams["project"]
	builder := c.URLParams["builder"]
	commit := c.URLParams["commit"]
	slave := c.URLParams["slave"]

	stdout, stderr, err := self.buildOutputReader.GetOutputStreamContent(project, builder, commit, slave)
	if err != nil {
		v.Feedback.Error("Error", fmt.Sprintf("Failed to create build output %s (builder: %s, commit: %s, slave: %s): %s", project, builder, commit, slave, err))
	}

	result := struct {
		Title    template.HTML
		Subtitle template.HTML
		Stderr   template.HTML
		Stdout   template.HTML
	}{
		Title:    template.HTML(fmt.Sprintf("%s", project)),
		Subtitle: template.HTML(fmt.Sprintf("<b>Builder:</b> %s, <b>Slave:</b> %s, <b>Commit</b>: %s", builder, slave, commit)),
		Stderr:   template.HTML(string(stderr)),
		Stdout:   template.HTML(string(stdout)),
	}

	v.Render(w, "project/buildoutput", result)
	return
}

func (self *webViewer) showProject(c web.C, w http.ResponseWriter, r *http.Request) {
	v := NewView()

	projectName := c.URLParams["id"]
	builds, err := presenter.CreateBuildResultListing(projectName, self.buildRepo)
	if err != nil {
		v.Feedback.Error("Error", fmt.Sprintf("Failed to create build results listing %s: %s", projectName, err))
		v.Render(w, "project/detail", nil)
		return
	}

	project := self.projectRepo.FindProject(projectName)
	if project == nil {
		v.Feedback.Error("Error", fmt.Sprintf("Project %s not found", projectName))
		v.Render(w, "project/detail", nil)
		return
	}

	// holds all the builds per commit
	var buildsByCommit []*CommitBuilds

	sort.Sort(viewmodel.BuildInfoSortByDate(builds))
	
	// finding all the builds which belong to a commit
	for _, b := range builds {
		if idx := findBuildIndex(buildsByCommit, b.Commit); idx >= 0 {
			buildsByCommit[idx].Builds = append(buildsByCommit[idx].Builds, b)
			continue
		}
		buildsByCommit = append(buildsByCommit, &CommitBuilds{Commit: b.Commit, Builds: []*viewmodel.BuildInfo{b}})
	}

	// creating the header row
	var headers []*viewmodel.HeaderRow

	for _, builder := range project.GetBuilder() {
		headers = append(headers, &viewmodel.HeaderRow{Name: builder.GetName(), NumSlaves: len(builder.GetSlave())})
	}

	var columnBuilderSlave []string

	var buildSlaveRow [][]string

	var row []string
	for _, builder := range project.GetBuilder() {
		for _, slave := range builder.GetSlave() {
			columnBuilderSlave = append(columnBuilderSlave, builder.GetName()+slave)
			row = append(row, slave)
		}
	}
	buildSlaveRow = append(buildSlaveRow, row)

	var buildRows [][]*viewmodel.BuildInfo
	for _, buildinfo := range buildsByCommit {
		var buildRow []*viewmodel.BuildInfo
		for i := 0; i < len(columnBuilderSlave); i++ {
			if found := FindBuildByKey(columnBuilderSlave[i], buildinfo.Builds); found != nil {
				buildRow = append(buildRow, found)
			} else {
				buildRow = append(buildRow, &viewmodel.BuildInfo{Status: "Unknown", StatusClass: "unknown"})
			}
		}
		buildRows = append(buildRows, buildRow)
	}

	result := struct {
		Title   string
		Content template.HTML
	}{
		Title:   projectName,
		Content: template.HTML(tmpl.Project_detail(headers, columnBuilderSlave, buildSlaveRow, buildRows)),
	}

	v.Render(w, "project/detail", result)
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
