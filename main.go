package main

import (
	"embed"
	"encoding/json"
	"errors"
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
)

//go:embed web
var memfs embed.FS
type microFS string
var downloader = http.FileServer(microFS(""))
var filesResource = flag.String("files", "./", "--files=./resource")
var controlScript = flag.String("script", "", "--script=./control.sh, execute demo: ./control.sh /files abc.txt")

func main() {
	flag.Parse()
	http.HandleFunc("/", DownloadHandler)
	http.HandleFunc("/scan", ScanHandler)
	log.Println("microweb ListenAndServe at 3535......")
	log.Println(http.ListenAndServe(":3535", nil))
}

func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Download %s ===> RequestURI: %s\n", r.URL.Path, r.RequestURI)
	downloader.ServeHTTP(w, r)
	return
}

func (p microFS) Open(name string) (http.File, error) {
	workDir, _ := os.Getwd()
	if strings.HasPrefix(name, "/" + filepath.Base(workDir)) {
		name = strings.TrimPrefix(name, "/" + filepath.Base(workDir))
	}
	fpath := *filesResource + strings.TrimPrefix(name, "/")

	if _, err := os.Stat(fpath); err != nil {
		return http.FS(memfs).Open("web/" + strings.TrimPrefix(name, "/"))
	} else {
		if *controlScript != "" {
			output, _ := exec.Command(*controlScript, filepath.Dir(fpath), filepath.Base(fpath)).Output()
			if strings.HasPrefix(string(output), "false:") {
				return nil, errors.New(strings.TrimPrefix(string(output), "false:"))
			}
		}
		return http.Dir("").Open(fpath)
	}
}

func ScanHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	directory := r.URL.Query().Get("directory")
	if directory == "" {
		directory, _ = os.Getwd()
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
	log.Printf("%s %s ===> Content-Type: %s, Content-Length: %d\n", r.Method, r.URL.Path, strType, len(data))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Type", strType)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}