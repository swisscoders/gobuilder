
GoBuilder is an open-source continuous integration framework inspired by [Buildbot](http://buildbot.net/). 

GoBuilder is cross platform since it's written in [Go](http://www.golang.org/)

**The project is in the alpha state and not production ready. It does work with Git only!**

## Purpose

The goal of this project is to make the automated build, test and release process easier and flexible. 

We believe that everyone should have the right and access to improve the overall software quality by using a CI framework 
(without needing to use a paid/hosted-solution or going through a painful, tedious and annoying installation & setup experiences).

## Getting started
First clone the project and then build the binaries from source codes:

**master:**
```
$ go build master.go
```

**slave:**
```
$ go build slave.go
```

**Source notifier hook:**
```
$ cd hooks
$ go build postreceive.go
```
**Content of the post-receive hook:**
```
#!/bin/sh
/path-to/postreceive -v=5 -logtostderr -address="master-address:3900" -project="MyProjectName" -repo="/path-to/git-repository"
```

**viewer:**
```
$ cd viewer
$ go build main.go
```
(At a later stage, we will make the build and installation process easier)

Please use the flag -h to get more information about how to use the binaries.

In order to start the build master you need to have a build config. Here is a sample:
```
project {
    name: "Sample"
    
    source {
        [proto.SourceNotifier.source] {
            address: "127.0.0.1:1234"
        }
    }

    scheduler {
        [proto.PeriodicScheduler.scheduler] {
            interval: 1
        }
        builder: "builder go test"
    }

    builder {
      name: "builder go test"
      blueprint: "sampleBlueprint"

      slave: "osx1"
      slave: "osx2"

      notifier {
        [proto.EmailNotifier.notifier] {
            from: "gobuilder@domain.tld"

            smtp {
                address: "smtp.domain.tld"
                port: 587
                username: "your-email@domain.tld"
                password: "your-password"
                
                cc: "any-other-email@domain.tld"
            }
        }
      }
    }

    blueprint {
        name: "sampleBlueprint"
        step {
            argv: "git" argv: "clone" argv: "$repo"
        }
        
        step {
            argv: "go" argv: "test" argv:"./..."
        }
        
    }
}

slave {
    name: "osx1"
    tag: "macbook"
    address: "localhost:50051"
}

slave {
    name: "osx2"
    tag: "macbook"
    address: "localhost:50052"
}
```
(Note: tags aren't implemented yet)

**Following variables are supported by now:**
* \$project -> Project name
* \$repo -> Git repository path
* \$commit -> Commit hash
* \$branch -> Name of the branch

## Contribute to the project - help us to make GoBuilder
We would love to get your help. The following areas need some improvements:

* Using the project and sending us feedback (filling in bug reports and fixing them, sending us suggestions, etc.)
* Writing unit tests
* Writing a documentation
* Logo

Please head over to the "Issues" and see what's the current status and how you can help us. If you are not sure then please do ask us.

## License
The **entire project** is licensed under the MIT License:

The MIT License (MIT)

Copyright (c) 2015 GoBuilder Team

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## GoBuilder Team
René Nussbaumer (Developer, maintainer)
Andreas Näpflin (Developer, maintainer)