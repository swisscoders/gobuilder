package controller

import (
	"net/http"
	"time"

	"../usecase"
	"github.com/zenazn/goji/web"
)

// BuildPlanInteractor usecase.BuildPlanInteractor

type ProjectController struct {
	ProjectInteractor usecase.ProjectInteractor
}

func (self *ProjectController) Index(c web.C, w http.ResponseWriter, r *http.Request) {
	type viewData struct {
		Key         string
		Repo        string
		Description string

		LatestBuildAt time.Time
		Status        string
	}

	v := NewView()
	projects, err := self.ProjectInteractor.ListAll()
	if err != nil {
		v.Feedback.Error("Error", err.Error())
	}

	var data []*viewData
	for _, p := range projects {
		d := &viewData{
			Key:           p.Key,
			Repo:          p.RepoPath,
			Description:   p.Description,
			Status:        "Never built",
			LatestBuildAt: time.Now(),
		}
		data = append(data, d)
	}

	v.Render(w, "project/index", data)
}

func (self *ProjectController) NewProject(c web.C, w http.ResponseWriter, r *http.Request) {
	v := NewView()
	v.Render(w, "project/new", nil)
}

func (self *ProjectController) SaveProject(c web.C, w http.ResponseWriter, r *http.Request) {
	v := NewView()

	err := self.ProjectInteractor.Save(r.FormValue("key"),
		r.FormValue("desc"),
		r.FormValue("repo"))

	if err != nil {
		v.Feedback.Error("Failed to save", err.Error())
	} else {
		v.Feedback.Success("Project stored", "Congrats you have stored the project. Add a new one below:")
	}

	v.Render(w, "project/new", nil)
}

func (self *ProjectController) EditTasks(c web.C, w http.ResponseWriter, r *http.Request) {
	type data struct {
		ProjectName string
	}

	NewView().Render(w, "project/tasks/edit", &data{c.URLParams["name"]})
}
