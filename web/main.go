package main

import (
	"net/http"

	"github.com/zenazn/goji"

	"./controller"

	"./interfaces"
	"./usecase"
)

func main() {
	projectInteractor := usecase.NewProjectInteractor(interfaces.NewInMemoryProjectsDb())
	projectInteractor.Save("ProjectA", "Project A Sample Repository", "sample-repo.git")
	projectInteractor.Save("gobuilder-master", "gobuilder master repo", "master-gobuilder.git")
	projectInteractor.Save("gobuilder-slave", "gobuilder slave repo", "slave-gobuilder.git")
	projectInteractor.Save("gobuilder-web", "gobuilder web frontend repo", "web-gobuilder.git")

	projCtrl := new(controller.ProjectController)
	projCtrl.ProjectInteractor = projectInteractor

	buildPlanInteractor := usecase.NewBuildPlanInteractor(interfaces.NewInMemoryBuildPlanDb())
	buildPlanInteractor.Save("gobuild", "build go only")
	buildPlanInteractor.Save("gobuild+test", "build go and test it")
	buildPlanInteractor.Save("gofmt+build+test", "format source code, build go and test it")

	buildPlanCtrl := new(controller.BuildPlanController)
	buildPlanCtrl.BuildPlanInteractor = buildPlanInteractor

	goji.Get("/assets/*", http.StripPrefix("/assets/", http.FileServer(http.Dir("public"))))

	goji.Get("/", projCtrl.Index)

	goji.Get("/project/:name/tasks", projCtrl.EditTasks)
	goji.Get("/project/new", projCtrl.NewProject)
	goji.Post("/project/new", projCtrl.SaveProject)

	goji.Get("/buildplan/index", buildPlanCtrl.Index)
	goji.Get("/buildplan/new", buildPlanCtrl.New)
	goji.Post("/buildplan/new", buildPlanCtrl.Save)

	goji.Get("/buildplan/:taskname/tasks", buildPlanCtrl.EditTasks)
	goji.Post("/buildplan/:taskname/save", buildPlanCtrl.SaveTask)

	goji.Serve()
}
