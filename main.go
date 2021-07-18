package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed web
var memfs embed.FS
type microFS string
var downloader = http.FileServer(microFS(""))
var listenPort = flag.String("port", "3535", "--port=3535")
var filesResource = flag.String("files", "./", "--files=./resource")
var visitAuthPath = flag.String("vpath", "", "--vpath=./resource")
var visitAuthCode = flag.String("vcode", "", "--vcode=xxxxxxxxxxxx")
var visitAuthDesc = flag.String("vauth", "", "--vauth=Please input password for authentication")
var controlScript = flag.String("script", "", "--script=./control.sh, execute demo: ./control.sh abc.txt xxxxxx")

func main() {
	flag.Parse()
	http.HandleFunc("/", DownloadHandler)
	http.HandleFunc("/scan", ScanHandler)
	http.HandleFunc("/visit", VisitHandler)
	log.Printf("microweb ListenAndServe at %s......\n", *listenPort)
	log.Println(http.ListenAndServe(":" + *listenPort, nil))
}

func VisitHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpResponse(w, r, []byte(err.Error()), "text/plain")
		return
	}
	hash := r.URL.Query().Get("hash")
	if strings.HasSuffix(hash, *visitAuthPath) && *visitAuthPath != "" {
		httpResponse(w, r, []byte(*visitAuthDesc), "text/plain")
		return
	}
	httpResponse(w, r, []byte(""), "text/plain")
}

func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Download %s ===> RequestURI: %s\n", r.URL.Path, r.RequestURI)
	if _, err := os.Stat("." + r.URL.Path); err == nil && *controlScript != "" {
		if err := r.ParseForm(); err != nil {
			httpResponse(w, r, []byte(err.Error()), "text/plain")
			return
		}

		output, _ := exec.Command("bash", *controlScript, filepath.Base(r.URL.Path), r.URL.Query().Get("code")).CombinedOutput()
		log.Printf("Control %s, %s ===> %s\n", filepath.Base(r.URL.Path), r.URL.Query().Get("code"), string(output))
		if strings.HasPrefix(string(output), "false:") {
			httpResponse(w, r, []byte(strings.TrimPrefix(string(output), "false:")), "text/plain")
			return
		} else if strings.HasPrefix(string(output), "redirect:") {
			http.Redirect(w, r, strings.TrimPrefix(string(output), "redirect:"), http.StatusFound)
			return
		} else if strings.HasPrefix(string(output), "path:") {
			fPath := strings.TrimSpace(strings.TrimPrefix(string(output), "path:"))
			fOut, err := os.Open(fPath)
			if err == nil {
				http.ServeContent(w, r, filepath.Base(fPath), time.Now(), fOut)
				go func() {
					<- time.After(time.Duration(1) * time.Hour)
					fOut.Close()
					os.RemoveAll(filepath.Dir(fPath))
				}()
				return
			} else {
				log.Println(err.Error())
			}
		}
	}
	downloader.ServeHTTP(w, r)
	return
}

func (p microFS) Open(name string) (http.File, error) {
	if name == "/" {
		return http.FS(memfs).Open("web")
	}
	if _, err := memfs.Open("web/" + strings.TrimPrefix(name, "/")); err == nil {
		return http.FS(memfs).Open("web/" + strings.TrimPrefix(name, "/"))
	}

	workDir, _ := filepath.Abs(*filesResource)
	if *filesResource == "" || *filesResource == "./" {
		workDir, _ = os.Getwd()
	}
	if strings.HasPrefix(name, "/" + filepath.Base(workDir)) {
		name = strings.TrimPrefix(name, "/" + filepath.Base(workDir))
	}
	log.Println(filepath.Join(workDir, strings.TrimPrefix(name, "/")))
	return http.Dir(workDir).Open("./" + strings.TrimPrefix(name, "/"))
}

func ScanHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpResponse(w, r, []byte(err.Error()), "text/plain")
		return
	}
	if *visitAuthCode != "" && *visitAuthCode != r.URL.Query().Get("code") {
		httpResponse(w, r, []byte("permission denied"), "text/plain")
		return
	}
	directory := r.URL.Query().Get("directory")
	if directory == "" {
		directory = *filesResource
		if directory == "./" {
			directory, _ = os.Getwd()
		}
	}

	ret := make(map[string]interface{}, 0)
	ret["type"] = "folder"
	ret["name"] = filepath.Base(directory)
	ret["path"] = filepath.Base(directory)
	ret["items"] = make([]interface{}, 0)

	ret, _ = readDir(directory, filepath.Dir(directory)+"/", ret)
	buf, _ := json.Marshal(ret)
	httpResponse(w, r, buf, "application/json")
}

func readDir(dirPath, trimPrefix string, parentMap map[string]interface{}) (map[string]interface{}, error) {
	flist, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("readDir failed, dirPath: %s, detail: %s", dirPath, err.Error())
	}

	for _, f := range flist {
		if f.IsDir() {
			var subErr error
			subMap := make(map[string]interface{}, 0)
			if subMap, subErr = readDir(dirPath+"/"+f.Name(), trimPrefix, subMap); subErr != nil {
				return nil, subErr
			} else {
				subMap["type"] = "folder"
				subMap["name"] = f.Name()
				subMap["size"] = f.Size()
				subMap["path"] = strings.TrimPrefix(dirPath+"/"+f.Name(), trimPrefix)
				if _, ok := subMap["items"]; !ok {
					subMap["items"] = make([]interface{}, 0)
				}

				if _, ok := parentMap["items"]; !ok {
					parentMap["items"] = make([]interface{}, 0)
				}
				items := parentMap["items"].([]interface{})
				items = append(items, subMap)
				parentMap["items"] = items
			}
		} else {
			subMap := make(map[string]interface{}, 0)
			subMap["type"] = "file"
			subMap["name"] = f.Name()
			subMap["size"] = f.Size()
			subMap["path"] = strings.TrimPrefix(dirPath+"/"+f.Name(), trimPrefix)
			if _, ok := parentMap["items"]; !ok {
				parentMap["items"] = make([]interface{}, 0)
			}
			items := parentMap["items"].([]interface{})
			items = append(items, subMap)
			parentMap["items"] = items
		}
	}
	return parentMap, nil
}

func httpResponse(w http.ResponseWriter, r *http.Request, data []byte, strType string) {
	log.Printf("%s %s ===> Content-Type: %s, Content-Length: %d\n", r.Method, r.RequestURI, strType, len(data))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Type", strType)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}