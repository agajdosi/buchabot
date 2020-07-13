package unslave

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
)

//Unslave removes all the mentions of master-slave terminology from the repository
//and replaces them with less disgusting terms.
func Unslave(workTree *git.Worktree) error {
	fmt.Println(" > unslaving")

	w := &walker{
		workTree: workTree,
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

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf(" >> error reading the file: %v", err)
	}

	text := string(data)
	if !(strings.Contains(text, "master") || strings.Contains(text, "slave")) {
		return nil
	}

	text = strings.Replace(text, "master", "oligarch", -1)
	text = strings.Replace(text, "slave", "politician", -1)

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
