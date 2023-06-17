package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/go-playground/validator/v10"
)

type ManifestMod struct {
	ProjectID int  `json:"projectID,omitempty" validate:"required"`
	FileID    int  `json:"fileID,omitempty" validate:"required"`
	Required  bool `json:"required,omitempty" validate:"required"`
}

type ManifestFile struct {
	Files []ManifestMod `json:"files,omitempty" validate:"required"`

	Version string `json:"version,omitempty" validate:"required"`
	Author  string `json:"author,omitempty" validate:"required"`
	Name    string `json:"name,omitempty" validate:"required"`
}

type DownloadUrlReq struct {
	Data string `json:"data,omitempty" validate:"required"`
}

const (
	initStr = `+--------------------------------------------------+
| Curseforge Downloader by Izan Rodrigues <zanz1n> |
| https://github.com/zanz1n/curseforge-downloader  |
+--------------------------------------------------+
`
	zeroStr = "                                                               "
)

var (
	validate = validator.New()

	startTime int64

	logMu = sync.Mutex{}

	fileName = flag.String("file-path", "./manifest.json", "The manifest filename")
	apiKey   = flag.String("api-key", "", "The curseforge api key")
	outPath  = flag.String("out", "./mods", "The mods outpur directory")
)

func fatal(v ...any) {
	fmt.Println(v...)
	os.Exit(1)
}

func twoDigit(x int) string {
	if x > 9 {
		return strconv.Itoa(x)
	}
	return "0" + strconv.Itoa(x)
}

func timeFmt(s int64) string {
	hours := math.Floor(float64(s) / (60 * 60))
	minutes := math.Floor(float64(s)/60 - (hours * 60))
	seconds := math.Floor(float64(s) - (minutes * 60) - (hours * 60))

	f := "[" +
		twoDigit(int(hours)) + ":" +
		twoDigit(int(minutes)) + ":" +
		twoDigit(int(seconds)) +
		"]"

	return f
}

func log(format string, v ...any) {
	logMu.Lock()
	defer logMu.Unlock()

	fmt.Print("\r" + zeroStr)

	fmt.Printf("\r%s\t%s\n",
		timeFmt(time.Now().Unix()-startTime),
		fmt.Sprintf(format, v...),
	)
}

func createFile(path string) (*os.File, error) {
	file, err := os.Open(path)

	if err == nil {
		file.Close()
		os.Remove(path)
	}

	return os.Create(path)
}

func capitalizeFirst(str string) string {
	if len(str) <= 0 {
		return str
	}
	rns := []rune(str)
	rns[0] = unicode.ToUpper(rns[0])
	return string(rns)
}

func percentString(n int) (s string) {
	n = n / 2
	s = "["
	for i := 0; i < 50; i++ {
		if n >= i {
			s = s + "#"
		} else {
			s = s + "-"
		}
	}
	s = s + "]"
	return
}

func printPercentage(i, total float64, canceled bool) {
	logMu.Lock()
	defer logMu.Unlock()

	percent := (i / total) * 100
	if canceled {
		fmt.Printf("\r%s CANCELED", percentString(int(percent)))
	} else {
		fmt.Printf("\r%s %v%s/100%s",
			percentString(int(percent)),
			int(percent),
			"%", "%",
		)
	}
}

func downloadFile(uri string) error {
	req, err := http.NewRequest("GET", uri, nil)

	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		return err
	}

	reqPathS := strings.Split(uri, "/")

	fileName := reqPathS[len(reqPathS)-1]

	filePath := *outPath + "/" + fileName

	file, err := createFile(filePath)

	if err != nil {
		return err
	}
	defer file.Close()

	if _, err = io.Copy(file, res.Body); err != nil {
		return err
	}

	return nil
}

func handleMod(mod ManifestMod) (int, error) {
	reqStr := fmt.Sprintf(
		"https://api.curseforge.com/v1/mods/%v/files/%v/download-url",
		mod.ProjectID,
		mod.FileID,
	)

	req, err := http.NewRequest("GET", reqStr, nil)

	if err != nil {
		return 0, err
	}

	req.Header.Add("content-type", "application/json")
	req.Header.Add("accepts", "application/json")

	req.Header.Add("x-api-key", *apiKey)

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		return 0, err
	}

	if res.StatusCode != 200 {
		return res.StatusCode, errors.New("request failed")
	}

	bodyBlob, err := io.ReadAll(res.Body)
	defer res.Body.Close()

	if err != nil {
		return 0, err
	}

	downloadUriBody := DownloadUrlReq{}

	json.Unmarshal(bodyBlob, &downloadUriBody)

	if err = validate.Struct(downloadUriBody); err != nil {
		return 0, errors.New("failed to parse the download-uri body")
	}

	if err = downloadFile(downloadUriBody.Data); err != nil {
		return 0, err
	}

	return 200, nil
}

func fileJob(i int, total float64, mod ManifestMod, endCh chan struct{}) {
	if st, err := handleMod(mod); err != nil {
		_ = st
		if st >= 400 {
			printPercentage(float64(i)+1, total, true)
			fmt.Print("\n")
			fatal("The authorization failed during one request, please review your api key")
		}
		log("%s", capitalizeFirst(err.Error()))
	}
	endCh <- struct{}{}
}

func init() {
	flag.Parse()

	if *apiKey == "" {
		*apiKey = os.Getenv("CURSEFORGE_API_KEY")

		if *apiKey == "" {
			fatal("A valid curseforge api key must be provided")
		}
	}
}

func main() {
	startTime = time.Now().Unix()
	fmt.Print(initStr + "\n")
	fileBuf, err := os.Open(*fileName)

	openErrStr := "Failed to open manifest file " + *fileName

	if err != nil {
		fatal(openErrStr)
	}

	fileBlob, err := io.ReadAll(fileBuf)

	if err != nil {
		fatal(openErrStr)
	}

	var file ManifestFile

	if err = json.Unmarshal(fileBlob, &file); err != nil {
		fatal(openErrStr)
	}

	if err = validate.Struct(file); err != nil {
		fatal(openErrStr)
	}

	fmt.Printf("Downloading modpack '%s' by '%s'\n\n", file.Name, file.Author)

	printPercentage(0, 100, false)

	total := float64(len(file.Files))

	endCh := make(chan struct{})

	for i, mod := range file.Files {
		log("File %v Download started", i+1)
		printPercentage(0, 100, false)
		go fileJob(i, total, mod, endCh)
	}

	i := 0

	for {
		<-endCh
		log("File %v Ok", i+1)
		printPercentage(float64(i)+1, total, false)
		i++
		if i == int(total)-1 {
			break
		}
	}
	printPercentage(float64(i)+1, total, false)

	fmt.Print("\n\n")

	fmt.Println(timeFmt(time.Now().Unix()-startTime) + "\tDownload completed")
}
