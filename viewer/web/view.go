package web

import (
	"fmt"
	"html/template"
	"io"
)

var t *template.Template

func setup() {
	tmpl := template.New("store")
	tmpl.Delims("[[", "]]")
	t = template.Must(tmpl.ParseGlob("web/view/*.html"))
}

type Feedback struct {
	IsInfo    bool
	IsError   bool
	IsSuccess bool

	Title   string
	Message string
}

func (self *Feedback) Error(title string, message string) {
	self.IsError = true
	self.Title = title
	self.Message = message
	return
}

func (self *Feedback) Info(title string, message string) {
	self.IsInfo = true
	self.Title = title
	self.Message = message
	return
}

func (self *Feedback) Success(title string, message string) {
	self.IsSuccess = true
	self.Title = title
	self.Message = message
	return
}

type View struct {
	Feedback  Feedback
	ShowNavig bool

	Content interface{}
	Vars    map[string]interface{}
}

func NewView() *View {
	return &View{Vars: make(map[string]interface{})}
}

func (self *View) Render(w io.Writer, name string, content interface{}) {
	setup()
	self.Content = content
	err := t.ExecuteTemplate(w, name, self)
	fmt.Println(err)
}
