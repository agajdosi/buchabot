package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/agajdosi/buchabot/unslave"
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

	searchRepos(ctx, client)
}

func searchRepos(ctx context.Context, client *github.Client) error {
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		//TODO: slowly search hour by hour to really get aaaaaall the results
		//results, resp, err := client.Search.Code(ctx, "master created:2020-07-07T12", opts)
		// OR BY SIZE!!!!
		//results, resp, err := client.Search.Code(ctx, "master size:2000..3000", opts)

		results, resp, err := client.Search.Code(ctx, "unhuman resources", opts)

		fmt.Println(*results.Total, *resp, err)

		if err != nil {
			return err
		}

		for _, result := range results.CodeResults {
			fixRepository(ctx, result.Repository, client)
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return nil
}

func fixExists(ctx context.Context, repository *github.Repository, client *github.Client) bool {
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	search := fmt.Sprintf("repo:%s author:%s", *repository.FullName, "agajdosi")
	results, _, _ := client.Search.Issues(ctx, search, opts)

	if len(results.Issues) == 0 {
		return false
	}

	return true
}

func fixRepository(ctx context.Context, repository *github.Repository, client *github.Client) error {
	fmt.Println("repository:", *repository.FullName)
	if fixExists(ctx, repository, client) {
		return nil
	}

	//here do the fix
	//maybe call a different module

	unslave.Unslave()

	return nil
}
