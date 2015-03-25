// contains application specific business rules
// http://manuel.kiessling.net/2012/09/28/applying-the-clean-architecture-to-go-applications/
package usecase

import (
	"fmt"

	"../domain"
)

type ProjectInteractor interface {
	Save(key, desc, repo string) error
	FindByKey(key string) (*domain.Project, error)
	ListAll() ([]*domain.Project, error)
}

type projectInteractor struct {
	ProjectRepository domain.ProjectRepository
}

func NewProjectInteractor(projectRepository domain.ProjectRepository) ProjectInteractor {
	return &projectInteractor{ProjectRepository: projectRepository}
}

func (self *projectInteractor) Save(key, desc, repo string) error {
	if key == "" {
		return fmt.Errorf("Key must not be empty")
	}

	return self.ProjectRepository.Store(&domain.Project{Key: key, Description: desc, RepoPath: repo})
}

func (self *projectInteractor) FindByKey(key string) (*domain.Project, error) {
	if key == "" {
		return &domain.Project{}, fmt.Errorf("Key must not be empty")
	}
	return self.ProjectRepository.FindByKey(key)
}

func (self *projectInteractor) ListAll() ([]*domain.Project, error) {
	return self.ProjectRepository.ListAll()
}

type BuildPlanInteractor interface {
	Save(key, description string) error
	FindByKey(key string) (*domain.BuildPlan, error)
	ListAll() ([]*domain.BuildPlan, error)
}

type buildPlanInteractor struct {
	BuildPlanRepository domain.BuildPlanRepository
}

func NewBuildPlanInteractor(buildPlanRepo domain.BuildPlanRepository) BuildPlanInteractor {
	return &buildPlanInteractor{BuildPlanRepository: buildPlanRepo}
}

func (self *buildPlanInteractor) Save(key, description string) error {
	if key == "" {
		return fmt.Errorf("Key must not be empty")
	}

	return self.BuildPlanRepository.Store(&domain.BuildPlan{Key: key, Description: description})
}

func (self *buildPlanInteractor) FindByKey(key string) (*domain.BuildPlan, error) {
	if key == "" {
		return nil, fmt.Errorf("Key must not be empty")
	}

	return self.BuildPlanRepository.FindByKey(key)
}

func (self *buildPlanInteractor) ListAll() ([]*domain.BuildPlan, error) {
	return self.BuildPlanRepository.ListAll()
}
