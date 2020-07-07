package main

import (
	"context"
	"flag"
	"fmt"

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

	opts := &github.SearchOptions{
		Sort: "created", Order: "asc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	results, resp, err := client.Search.Code(ctx, "slave", opts)

	if err != nil {
		fmt.Printf("\nerror: %v\n", err)
		return
	}

	fmt.Println(resp, err)

	for _, result := range results.CodeResults {
		fixRepository(result.Repository)
	}

}

func fixRepository(repository *github.Repository) error {
	if fixExists(repository) {
		return nil
	}

	fmt.Println(*repository.FullName)
	return nil
}

func fixExists(repository *github.Repository) bool {
	return false
}
