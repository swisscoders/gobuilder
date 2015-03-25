package controller

import (
	"encoding/json"
	"fmt"
	"net/http"

	"../usecase"
	"github.com/kr/pretty"
	"github.com/zenazn/goji/web"
)

type BuildPlanController struct {
	BuildPlanInteractor usecase.BuildPlanInteractor
}

func (self *BuildPlanController) New(c web.C, w http.ResponseWriter, r *http.Request) {
	v := NewView()
	v.Render(w, "buildplan/new", nil)
}

func (self *BuildPlanController) Save(c web.C, w http.ResponseWriter, r *http.Request) {
	v := NewView()

	err := self.BuildPlanInteractor.Save(r.FormValue("key"), r.FormValue("desc"))

	if err != nil {
		v.Feedback.Error("Failed to save", err.Error())
	} else {
		v.Feedback.Success("Buildplan stored", "Congrats you have stored the build plan. Add a new one below:")
	}

	v.Render(w, "buildplan/new", nil)
}

func (self *BuildPlanController) Index(c web.C, w http.ResponseWriter, r *http.Request) {
	type viewData struct {
		Key         string
		Description string
	}

	v := NewView()
	buildplans, err := self.BuildPlanInteractor.ListAll()
	if err != nil {
		v.Feedback.Error("Error", err.Error())
	}

	var data []*viewData
	for _, bp := range buildplans {
		d := &viewData{
			Key:         bp.Key,
			Description: bp.Description,
		}
		data = append(data, d)
	}

	v.Render(w, "buildplan/index", data)
}

func (self *BuildPlanController) EditTasks(c web.C, w http.ResponseWriter, r *http.Request) {
	type viewData struct {
		BuildPlanName string
	}

	v := NewView()
	v.Render(w, "buildplan/tasks", &viewData{BuildPlanName: c.URLParams["taskname"]})
}

func (self *BuildPlanController) SaveTask(c web.C, w http.ResponseWriter, r *http.Request) {
	tasks := []byte(r.FormValue("buildtasks"))
	fmt.Println(r.FormValue("buildtasks"))
	var dat JsonTasks
	if err := json.Unmarshal(tasks, &dat); err != nil {
		panic(err)
	}
	pretty.Println(dat)
}

type JsonTasks struct {
	Commands []*JsonTask `json:"commands"`
}

type JsonTask struct {
	Command     string        `json:"cmd"`
	SubCommands [][]*JsonTask `json:"columns"`
}
