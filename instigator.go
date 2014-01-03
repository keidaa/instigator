package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
	"sort"

	"github.com/keidaa/llog"
	"github.com/russross/blackfriday"
)

var log = llog.New(os.Stdout, llog.DEBUG)

var config struct {
	SourceDir,
	TemplateDir,
	OutputDir string
}

type Post struct {
	Name,
	Title,
	Content string
	Date time.Time
}

type Posts []Post

func (p Posts) Len() int           { return len(p) }
func (p Posts) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p Posts) Less(i, j int) bool { return p[i].Date.After(p[j].Date) }

// parse markdown file and convert to html
func parseSourceFile(srcFilePath string) (*Post, error) {
	post := &Post{}

	post.Name = trimPath(srcFilePath)

	// date
	d, err := parseDate(post.Name)
	if err != nil {
		log.Warning(err)
	}
	post.Date = d

	// read file
	data, err := ioutil.ReadFile(srcFilePath)
	if err != nil {
		return nil, err
	}

	// parse title from first headline
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if s := strings.TrimLeft(line, " "); strings.HasPrefix(s, "#") {
			post.Title = strings.TrimLeft(strings.TrimLeft(s, "#"), " ")
			break
		}
	}

	// convert markdown to html
	content := strings.Join(lines, "\n")
	output := blackfriday.MarkdownCommon([]byte(content))
	post.Content = string(output)

	return post, nil
}

func parseDate(name string) (time.Time, error) {
	if r, err := regexp.Compile(`(\d{1,4})-(\d{1,2})-(\d{1,2})`); err == nil {
		// find date string
		ds := r.FindString(name)
		// quick check
		if len(ds) == 10 {
			if d, err := time.Parse("2006-01-02", ds); err == nil {
				return d, nil
			} else {
				return time.Now(), err
			}
		}
	} else {
		return time.Now(), err
	}
	return time.Now(), fmt.Errorf("Unable to parse date from string: %v", name)
}

func trimPath(path string) string {
	fn := filepath.Base(path)
	ext := filepath.Ext(fn)
	return strings.TrimRight(fn, ext)
}

func renderTemplate(tmplPath string, tmplData interface{}) ([]byte, error) {
	// read template
	data, err := ioutil.ReadFile(tmplPath)
	if err != nil {
		return nil, err
	}

	// parse template
	tmpl, err := template.New(tmplPath).Parse(string(data))
	if err != nil {
		return nil, err
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, tmplData); err != nil {
		return nil, err
	}

	return []byte(buffer.String()), nil
}

func writeOutputFile(outFilePath string, html []byte) error {
	// outfile := filepath.Join(config.OutputDir, strings.Join([]string{name, "html"}, "."))
	err := ioutil.WriteFile(outFilePath, html, 0644)
	if err != nil {
		return err
	}
	return nil
}

func readConfig() error {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(file, &config); err != nil {
		return err
	}
	return nil
}

func writeIndex(posts Posts) error {
	// sort posts
	sort.Sort(posts)

	// recent posts
	out, err := renderTemplate(filepath.Join(config.TemplateDir, "recent.html"), posts)
	if err != nil {
		return err
	}

	recent := struct {
		Title,
		Content string
	}{
		"my page",
		string(out),
	}

	// tuck recent into main template
	out, err = renderTemplate(filepath.Join(config.TemplateDir, "main.html"), recent)
	if err != nil {
		return err
	}

	if err := writeOutputFile(filepath.Join(config.OutputDir, "index.html"), out); err != nil {
		return err
	}

	return nil
}

func writePost(mdPath string) (*Post, error) {
	// parse post
	post, err := parseSourceFile(mdPath)
	if err != nil {
		return nil, err
	}

	// render template
	tmplPath := filepath.Join(config.TemplateDir, "main.html")
	out, err := renderTemplate(tmplPath, post)
	if err != nil {
		return nil, err
	}

	// write post
	outFilePath := filepath.Join(config.OutputDir, post.Name+".html")
	if err := writeOutputFile(outFilePath, out); err != nil {
		return nil, err
	}

	// returning post to be stored in Posts
	return post, nil
}

func writeFeed(posts Posts) error {
	// sort posts
	sort.Sort(posts)

	// rss
	out, err := renderTemplate(filepath.Join(config.TemplateDir, "feed.html"), posts)
	if err != nil {
		return err
	}

	if err := writeOutputFile(filepath.Join(config.OutputDir, "feed.html"), out); err != nil {
		return err
	}

	return nil
}

func listSrcFiles() ([]string, error) {
	return filepath.Glob(config.SourceDir + "/*.md")
}

func prepare() error {
	// add current date to source files if date not manually set
	srcFiles, err := listSrcFiles()
	if err != nil { 
		return err
	}

	for _, srcFile := range srcFiles {
		name := trimPath(srcFile)
		date, err := parseDate(name)
		// add current date if parsedate failed (meaning no date prefix in filename)
		if err != nil {
			dateStr := date.Format("2006-01-02")
			newname := filepath.Join(config.SourceDir, dateStr + "-" + name + filepath.Ext(srcFile))
			if err := os.Rename(srcFile, newname); err == nil {
				log.Debugf("Renamed %v to %v", srcFile, newname)
			} else {
				// we still want to return nil error since it's not fatal
				log.Error(err)
			}
		}
	}

	return nil
}

func main() {
	// read config
	if err := readConfig(); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	// prepare
	if err := prepare(); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	// collect source files
	srcFiles, err := listSrcFiles()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	// write posts
	posts := make(Posts, len(srcFiles))
	for i, srcFile := range srcFiles {
		if post, err := writePost(srcFile); err == nil {
			log.Info("Saved post: " + post.Name)
			posts[i] = *post
		} else { // error
			log.Error(err)
		}
	}

	// write index
	if err := writeIndex(posts); err == nil {
		log.Info("Saved index")
	} else { // error
		log.Error(err)
	}

	// write feed
	if err := writeFeed(posts); err == nil {
		log.Info("Saved feed")
	} else { // error
		log.Error(err)
	}
	fmt.Println()
}
