package main

import (
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"code.google.com/p/go.net/websocket"
	"github.com/shurcooL/Go-Package-Store/presenter"
	"github.com/shurcooL/go-goon"
	"github.com/shurcooL/go/exp/14"
	"github.com/shurcooL/go/gists/gist7480523"
	"github.com/shurcooL/go/gists/gist7651991"
	"github.com/shurcooL/go/gists/gist7802150"
	"github.com/shurcooL/go/u/u4"
	"github.com/shurcooL/gostatus/status"
)

func CommonHat(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	io.WriteString(w, `<html>
	<head>
		<title>Go Package Store</title>
		<link href="assets/style.css" rel="stylesheet" type="text/css" />
		<script src="assets/script.js" type="text/javascript"></script>
	</head>
	<body>
		<div style="width: 100%; text-align: center; background-color: hsl(209, 51%, 92%); border-bottom: 1px solid hsl(209, 51%, 88%);">
			<span style="background-color: hsl(209, 51%, 88%); padding: 15px; display: inline-block;">Updates</span>
		</div>
		<script type="text/javascript">var sock = new WebSocket("ws://localhost:7043/opened");</script>
		<div class="content">`)
}
func CommonTail(w io.Writer) {
	io.WriteString(w, `<div id="installed_updates" style="display: none;"><h3 style="text-align: center;">Installed Updates</h3></div>`)
	io.WriteString(w, "</div></body></html>")
}

// ---

func shouldPresentUpdate(goPackage *gist7480523.GoPackage) bool {
	return status.PlumbingPresenterV2(goPackage)[:3] == "  +" // Ignore stash.
}

func WriteRepoHtml(w http.ResponseWriter, repoPresenter presenter.Presenter) {
	err := t.Execute(w, repoPresenter)
	if err != nil {
		log.Println("t.Execute:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var goPackages exp14.GoPackageList = &exp14.GoPackages{SkipGoroot: true}

type updateRequest struct {
	importPathPattern string
	resultChan        chan error
}

var updateRequestChan = make(chan updateRequest)

func updateWorker() {
	for updateRequest := range updateRequestChan {
		fmt.Println("go", "get", "-u", "-d", updateRequest.importPathPattern)

		cmd := exec.Command("go", "get", "-u", "-d", updateRequest.importPathPattern)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Run()

		gist7802150.MakeUpdated(goPackages)
		for _, goPackage := range goPackages.List() {
			if rootPath := getRootPath(goPackage); rootPath != "" {
				if gist7480523.GetRepoImportPathPattern(rootPath, goPackage.Bpkg.SrcRoot) == updateRequest.importPathPattern {
					fmt.Println("ExternallyUpdated", updateRequest.importPathPattern)
					gist7802150.ExternallyUpdated(goPackage.Dir.Repo.VcsLocal.GetSources()[1].(gist7802150.DepNode2ManualI))
					break
				}
			}
		}

		updateRequest.resultChan <- err
	}
}

func updateHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		if *godepsFlag != "" {
			// TODO: Implement updating Godeps packages.
			log.Fatalln("updating Godeps packages isn't supported yet")
		}

		updateRequest := updateRequest{
			importPathPattern: req.PostFormValue("import_path_pattern"),
			resultChan:        make(chan error),
		}
		updateRequestChan <- updateRequest

		err := <-updateRequest.resultChan
		_ = err // Don't do anything about the error for now.
	}
}

func getRootPath(goPackage *gist7480523.GoPackage) (rootPath string) {
	if goPackage.Standard {
		return ""
	}

	goPackage.UpdateVcs()
	if goPackage.Dir.Repo == nil {
		return ""
	} else {
		return goPackage.Dir.Repo.Vcs.RootPath()
	}
}

func mainHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: When "finished", should not reload templates from disk on each request... Unless using a dev flag?
	if err := loadTemplates(); err != nil {
		fmt.Fprintln(w, "loadTemplates:", err)
		return
	}

	started := time.Now()

	CommonHat(w)
	defer CommonTail(w)

	io.WriteString(w, `<div id="checking_updates"><h2 style="text-align: center;">Checking for updates...</h2></div>`)
	io.WriteString(w, `<div id="no_updates" style="display: none;"><h2 style="text-align: center;">No Updates Available</h2></div>`)
	defer io.WriteString(w, `<script>document.getElementById("checking_updates").style.display = "none";</script>`)

	flusher := w.(http.Flusher)
	flusher.Flush()

	notifier := w.(http.CloseNotifier)
	go func() {
		<-notifier.CloseNotify()
		os.Exit(0)
	}()

	fmt.Printf("Part 1: %v ms.\n", time.Since(started).Seconds()*1000)

	// rootPath -> []*gist7480523.GoPackage
	var goPackagesInRepo = make(map[string][]*gist7480523.GoPackage)

	gist7802150.MakeUpdated(goPackages)
	fmt.Printf("Part 1b: %v ms.\n", time.Since(started).Seconds()*1000)
	if false {
		for _, goPackage := range goPackages.List() {
			if rootPath := getRootPath(goPackage); rootPath != "" {
				goPackagesInRepo[rootPath] = append(goPackagesInRepo[rootPath], goPackage)
			}
		}
	} else {
		inChan := make(chan interface{})
		go func() { // This needs to happen in the background because sending input will be blocked on reading output.
			for _, goPackage := range goPackages.List() {
				inChan <- goPackage
			}
			close(inChan)
		}()
		reduceFunc := func(in interface{}) interface{} {
			goPackage := in.(*gist7480523.GoPackage)
			if rootPath := getRootPath(goPackage); rootPath != "" {
				return gist7480523.NewGoPackageRepo(rootPath, []*gist7480523.GoPackage{goPackage})
			}
			return nil
		}
		outChan := gist7651991.GoReduce(inChan, 64, reduceFunc)
		for out := range outChan {
			repo := out.(gist7480523.GoPackageRepo)
			goPackagesInRepo[repo.RootPath()] = append(goPackagesInRepo[repo.RootPath()], repo.GoPackages()[0])
		}
	}

	goon.DumpExpr(len(goPackages.List()))
	goon.DumpExpr(len(goPackagesInRepo))

	fmt.Printf("Part 2: %v ms.\n", time.Since(started).Seconds()*1000)

	updatesAvailable := 0

	inChan := make(chan interface{})
	go func() { // This needs to happen in the background because sending input will be blocked on reading output.
		for rootPath, goPackages := range goPackagesInRepo {
			inChan <- gist7480523.NewGoPackageRepo(rootPath, goPackages)
		}
		close(inChan)
	}()
	reduceFunc := func(in interface{}) interface{} {
		repo := in.(gist7480523.GoPackageRepo)

		goPackage := repo.GoPackages()[0]
		goPackage.UpdateVcsFields()

		if !shouldPresentUpdate(goPackage) {
			return nil
		}
		repoPresenter := presenter.New(&repo)
		return repoPresenter
	}
	outChan := gist7651991.GoReduce(inChan, 8, reduceFunc)

	for out := range outChan {
		started2 := time.Now()

		repoPresenter := out.(presenter.Presenter)

		updatesAvailable++
		WriteRepoHtml(w, repoPresenter)

		flusher.Flush()

		fmt.Printf("Part 2b: %v ms.\n", time.Since(started2).Seconds()*1000)
	}

	if updatesAvailable == 0 {
		io.WriteString(w, `<script>document.getElementById("no_updates").style.display = "";</script>`)
	}

	fmt.Printf("Part 3: %v ms.\n", time.Since(started).Seconds()*1000)
}

func openedHandler(ws *websocket.Conn) {
	// Wait until connection is closed.
	io.Copy(ioutil.Discard, ws)

	os.Exit(0)
}

// ---

var t *template.Template

func loadTemplates() error {
	const filename = "./assets/repo.html.tmpl"

	var err error
	t, err = template.ParseFiles(filename)
	return err
}

var godepsFlag = flag.String("godeps", "", "Path to Godeps file to use.")

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	err := loadTemplates()
	if err != nil {
		log.Fatalln("loadTemplates:", err)
	}

	flag.Parse()
	if *godepsFlag != "" {
		fmt.Println("Using Godeps file:", *godepsFlag)
		goPackages = NewGoPackagesFromGodeps(*godepsFlag)
	}

	goon.DumpExpr(os.Getwd())
	goon.DumpExpr(os.Getenv("PATH"), os.Getenv("GOPATH"))

	http.HandleFunc("/index", mainHandler)
	http.HandleFunc("/-/update", updateHandler)
	http.Handle("/favicon.ico", http.NotFoundHandler())
	http.Handle("/assets/", http.FileServer(http.Dir(".")))
	http.Handle("/opened", websocket.Handler(openedHandler)) // Exit server when client tab is closed.
	go updateWorker()

	u4.Open("http://localhost:7043/index")

	err = http.ListenAndServe("localhost:7043", nil)
	if err != nil {
		panic(err)
	}
}
