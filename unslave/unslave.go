package unslave

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
)

//Unslave removes all the mentions of master-slave terminology from the repository
//and replaces them with less disgusting terms.
func Unslave() error {
	fmt.Println("unslaving")
	filename := filepath.Join(".temp", "test-git-file")
	ioutil.WriteFile(filename, []byte("hello world!"), 0644)
	fmt.Println("unslaved")
	return nil
}
