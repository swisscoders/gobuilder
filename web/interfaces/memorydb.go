package interfaces //or adapter

import (
	"fmt"

	"../domain"
)

type ProjectStorage interface {
	Store(*domain.Project) error
	DeleteByKey(key string) error
	FindByKey(key string) (*domain.Project, error)
	ListAll() (projects []*domain.Project, err error)
}

// Implementation:

func NewInMemoryProjectsDb() ProjectStorage {
	return &memoryProjectStorage{make(map[string]*domain.Project)}
}

type memoryProjectStorage struct {
	data map[string]*domain.Project
}

func (self *memoryProjectStorage) Store(p *domain.Project) error {
	self.data[p.Key] = p
	return nil
}

func (self *memoryProjectStorage) DeleteByKey(key string) error {
	delete(self.data, key)
	return nil
}

func (self *memoryProjectStorage) FindByKey(key string) (*domain.Project, error) {
	if p, has := self.data[key]; has {
		return p, nil
	}
	return nil, fmt.Errorf("%s not found", key)
}

func (self *memoryProjectStorage) ListAll() (projects []*domain.Project, err error) {
	for _, p := range self.data {
		projects = append(projects, p)
	}
	return
}

type BuildPlanStorage interface {
	Store(*domain.BuildPlan) error
	DeleteByKey(key string) error
	FindByKey(key string) (*domain.BuildPlan, error)
	ListAll() ([]*domain.BuildPlan, error)
}

func NewInMemoryBuildPlanDb() BuildPlanStorage {
	return &memoryBuildPlanStorage{make(map[string]*domain.BuildPlan)}
}

type memoryBuildPlanStorage struct {
	data map[string]*domain.BuildPlan
}

func (self *memoryBuildPlanStorage) Store(bp *domain.BuildPlan) error {
	self.data[bp.Key] = bp
	return nil
}

func (self *memoryBuildPlanStorage) DeleteByKey(key string) error {
	delete(self.data, key)
	return nil
}

func (self *memoryBuildPlanStorage) FindByKey(key string) (*domain.BuildPlan, error) {
	if v, has := self.data[key]; has {
		return v, nil
	}
	return nil, fmt.Errorf("%s not found", key)
}

func (self *memoryBuildPlanStorage) ListAll() (bps []*domain.BuildPlan, err error) {
	for _, item := range self.data {
		bps = append(bps, item)
	}
	return
}
