package unslave

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
)

//Unslave removes all the mentions of master-slave terminology from the repository
//and replaces them with less disgusting terms.
func Unslave(workTree *git.Worktree) error {
	fmt.Println(" > unslaving")

	w := &walker{
		workTree: workTree,
		exp:      regexp.MustCompile(`([\w-]*\.\w{2,4}\/[ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789\-\._~:/?#\[\]@!$&'()*+,;=]*)*([Ss]lave|[Mm]aster)`),
	}

	err := filepath.Walk(".temp", w.removeMasterSlave)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(" > unslaved")

	return nil
}

type walker struct {
	workTree *git.Worktree
	exp      *regexp.Regexp
}

func (w *walker) replace(text string) string {
	if text == "slave" {
		return "subordinate"
	}
	if text == "Slave" {
		return "Subordinate"
	}
	if text == "master" {
		return "main"
	}
	if text == "Master" {
		return "Main"
	}

	return text
}

func (w *walker) removeMasterSlave(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if info.IsDir() == true && info.Name() == ".git" {
		return filepath.SkipDir
	}

	if info.IsDir() == true {
		return nil
	}

	skip := map[string]bool{
		".jpg":  true,
		".png":  true,
		".db":   true,
		".html": true,
		".xml":  true,
	}
	if skip[filepath.Ext(info.Name())] {
		return nil
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf(" >> error reading the file: %v", err)
	}

	text := string(data)
	if !(strings.Contains(text, "master") || strings.Contains(text, "slave")) {
		return nil
	}

	text = w.exp.ReplaceAllStringFunc(text, w.replace)

	err = ioutil.WriteFile(path, []byte(text), 0644)
	if err != nil {
		return fmt.Errorf(" >> error writing to file: %v", err)
	}

	//STAGING THE FILE
	localPath := strings.SplitN(path, string(filepath.Separator), 2)[1]
	_, err = w.workTree.Add(localPath)
	if err != nil {
		return fmt.Errorf(" >> error staging file (%v): %v", localPath, err)
	}

	return err
}

//GeneratePRTitle generates title for the PR which removes master/slave terminology
func GeneratePRTitle() string {
	words := [][]string{
		{"Removal of", "Removing the", "Changing the", "Discarding the", "Discard", "Migrate from", "Migrating from", "Get away from"},
		{"master/slave", "master-slave", "master and slave"},
		{"terminology", "words", "jargon", "lingo", "vocabulary", "phrasing", "language"},
	}

	return fmt.Sprintf("%v %v %v", words[0][rand.Intn(len(words[0]))], words[1][rand.Intn(len(words[1]))], words[2][rand.Intn(len(words[2]))])
}

//GeneratePRDescription generates description for the PR which removes master/slave terminology
func GeneratePRDescription() string {
	return "For diversity reasons, it would be nice to try to avoid 'master' and 'slave' terminology in this repository which can be associated to slavery. The master-slave terminology could be problematic for people in several countries which has the history of slavery like Romania, USA and many others. Thank you for considering the proposal. Let me know if any changes in the PR are needed, I would be happy to implement them."
}
