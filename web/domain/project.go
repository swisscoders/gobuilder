// Domain entities encapsulate Enterprise wide business rules.
// can be an object with methods, or it can be a set of data structures and functions.
// It doesn’t matter so long as the domain entities
// could be used by many different applications in the enterprise.
// http://manuel.kiessling.net/2012/09/28/applying-the-clean-architecture-to-go-applications/
package domain

type ProjectRepository interface {
	Store(*Project) error
	FindByKey(key string) (*Project, error)
	ListAll() ([]*Project, error)
}

type Project struct {
	Key         string
	Description string
	RepoPath    string
}

type BuildPlanRepository interface {
	Store(*BuildPlan) error
	FindByKey(key string) (*BuildPlan, error)
	ListAll() ([]*BuildPlan, error)
}

type BuildPlan struct {
	Key         string
	Description string

	Tasks []*BuildTask
}

type BuildTask struct {
	Command string

	Children []*BuildTask
}
