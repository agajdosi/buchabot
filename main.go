package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/agajdosi/buchabot/unslave"

	"github.com/go-git/go-git/v5"
	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

func main() {
	token := flag.String("token", "", "GitHub Token which will be used to search code and place fixing commits.")
	flag.Parse()

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	searchAllRepos(ctx, client)
}

func searchAllRepos(ctx context.Context, client *github.Client) {
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 10},
	}

	for {
		resp, err := searchRepos(ctx, client, opts)
		if err != nil {
			fmt.Println(err)
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}
}

func searchRepos(ctx context.Context, client *github.Client, opts *github.SearchOptions) (*github.Response, error) {
	//TODO: slowly search hour by hour to really get aaaaaall the results
	//results, resp, err := client.Search.Code(ctx, "master created:2020-07-07T12", opts)
	// OR BY SIZE!!!!
	//results, resp, err := client.Search.Code(ctx, "master size:2000..3000", opts)
	results, resp, err := client.Search.Code(ctx, "slave", opts)
	handleAPILimit(resp)
	if err != nil {
		return resp, err
	}

	for _, result := range results.CodeResults {
		fixRepository(ctx, result.Repository, client)
	}

	return resp, nil
}

func handleAPILimit(response *github.Response) {
	if response.Rate.Remaining < 5 {
		length := time.Until(response.Rate.Reset.Time) + time.Duration(time.Second*5)
		time.Sleep(length)
	}
	return
}

func fixRepository(ctx context.Context, repository *github.Repository, client *github.Client) error {
	fmt.Printf("\n" + *repository.FullName + "\n")
	if fixExists(ctx, repository, client) {
		return nil
	}

	forkedRepo, _ := forkRepo(ctx, repository, client)

	err := cloneRepo(forkedRepo)
	if err != nil {
		return err
	}

	unslave.Unslave()

	return nil
}

func forkRepo(ctx context.Context, repository *github.Repository, client *github.Client) (*github.Repository, error) {
	opts := &github.RepositoryCreateForkOptions{}
	owner := *repository.Owner.Login
	name := repository.GetName()

	repo, resp, err := client.Repositories.CreateFork(ctx, owner, name, opts)
	handleAPILimit(resp)
	time.Sleep(time.Duration(time.Second * 10)) // it takes few seconds to GH to fork the repo

	return repo, err
}

func cloneRepo(repository *github.Repository) error {
	err := os.RemoveAll(".temp")
	if err != nil {
		fmt.Println("error deleting temp directory:", err)
	}

	_, err = git.PlainClone(".temp", false, &git.CloneOptions{
		URL:      *repository.CloneURL,
		Progress: os.Stdout,
	})

	fmt.Println("repository cloned")
	return err
}

func fixExists(ctx context.Context, repository *github.Repository, client *github.Client) bool {
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	search := fmt.Sprintf("repo:%s author:%s", *repository.FullName, "bopopescu")
	results, resp, _ := client.Search.Issues(ctx, search, opts)
	handleAPILimit(resp)

	if len(results.Issues) == 0 {
		return false
	}

	return true
}
