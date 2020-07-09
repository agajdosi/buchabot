package main

import (
	"context"
	"flag"
	"fmt"
	"time"

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

	fmt.Println(*results.Total, *resp, err)

	for _, result := range results.CodeResults {
		fixRepository(ctx, result.Repository, client)
	}

	return resp, nil
}

func handleAPILimit(response *github.Response) {
	if response.Rate.Remaining < 5 {
		fmt.Println("SLEEEEPING")
		length := time.Until(response.Rate.Reset.Time) + time.Duration(5*time.Second)
		time.Sleep(length)
	}
	return
}

func fixRepository(ctx context.Context, repository *github.Repository, client *github.Client) error {
	fmt.Println("repository:", *repository.FullName)
	if fixExists(ctx, repository, client) {
		return nil
	}

	//here do the fix
	//maybe call a different module

	//unslave.Unslave()

	return nil
}

func fixExists(ctx context.Context, repository *github.Repository, client *github.Client) bool {
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	search := fmt.Sprintf("repo:%s author:%s", *repository.FullName, "agajdosi")
	results, resp, err := client.Search.Issues(ctx, search, opts)
	handleAPILimit(resp)
	fmt.Println(*results.Total, *resp, err)

	if len(results.Issues) == 0 {
		return false
	}

	return true
}
