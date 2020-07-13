package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agajdosi/buchabot/unslave"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

func main() {
	token := flag.String("token", "", "GitHub Token which will be used to search code, fork repositories, place fixing commits and create PRs to original repositories.")
	email := flag.String("email", "", "Sets email which will be used in the commits. Please set this if your email is not publicly visible and token does not have access to email:list on GitHub.")
	flag.Parse()

	if *token == "" {
		fmt.Println("Token flag is required.")
		os.Exit(100)
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	user, _ := getUser(ctx, client, email)
	fmt.Println("email:", *user.Email)

	searchAllRepos(ctx, client, user, token)
}

func getUser(ctx context.Context, client *github.Client, email *string) (*github.User, error) {
	user, resp, err := client.Users.Get(ctx, "")
	handleAPILimit(resp)
	if err != nil {
		fmt.Println(err)
	}

	if *email != "" {
		user.Email = email
		return user, err
	}

	if user.GetEmail() != "" {
		return user, err
	}

	emails, resp, err := client.Users.ListEmails(ctx, &github.ListOptions{})
	handleAPILimit(resp)
	if err != nil {
		fmt.Println(err)
	}

	e := emails[0].GetEmail()
	user.Email = &e

	return user, err
}

func searchAllRepos(ctx context.Context, client *github.Client, user *github.User, token *string) {
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 10},
	}

	for {
		resp, err := searchRepos(ctx, client, opts, user, token)
		if err != nil {
			fmt.Println(err)
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}
}

func searchRepos(ctx context.Context, client *github.Client, opts *github.SearchOptions, user *github.User, token *string) (*github.Response, error) {
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
		fixRepository(ctx, result.Repository, client, user, token)
	}

	return resp, nil
}

func createPR(ctx context.Context, client *github.Client, repository *github.Repository, fork *github.Repository, gitRepo *git.Repository, token *string) error {
	title := "Small update"
	description := "please consider following changes"

	gitHead, err := gitRepo.Head()
	head := *fork.GetOwner().Login + ":" + gitHead.Name().Short()

	base := repository.GetDefaultBranch()
	if base == "" {
		base = "master"
	}

	newPR := &github.NewPullRequest{
		Title:               &title,
		Head:                &head,
		Base:                &base,
		Body:                &description,
		MaintainerCanModify: github.Bool(true),
	}

	_, resp, err := client.PullRequests.Create(ctx, *repository.Owner.Login, repository.GetName(), newPR)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(" > PR successfully created")
	}

	handleAPILimit(resp)
	return nil
}

func handleAPILimit(response *github.Response) {
	if response.Rate.Remaining < 5 {
		length := time.Until(response.Rate.Reset.Time) + time.Duration(time.Second*5)
		time.Sleep(length)
	}
	return
}

func fixRepository(ctx context.Context, repository *github.Repository, client *github.Client, user *github.User, token *string) error {
	fmt.Printf("\n" + *repository.FullName)
	if fixExists(ctx, repository, client) {
		return nil
	}

	fork, _ := forkRepo(ctx, repository, client)

	gitRepo, workTree, err := cloneRepo(fork)
	if err != nil {
		fmt.Println(err)
		return err
	}

	checkoutBranch(gitRepo, workTree)
	unslave.Unslave()
	commitChanges(gitRepo, workTree, user)
	pushChanges(gitRepo, token)
	createPR(ctx, client, repository, fork, gitRepo, token)

	return nil
}

func fixExists(ctx context.Context, repository *github.Repository, client *github.Client) bool {
	opts := &github.PullRequestListOptions{
		State: "all",
	}
	pullRequests, resp, _ := client.PullRequests.List(ctx, *repository.Owner.Login, *repository.Name, opts)
	handleAPILimit(resp)

	for _, pullRequest := range pullRequests {
		if strings.Contains(*pullRequest.Body, "removes master slave terminology") {
			return true
		}
	}

	return false
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

func cloneRepo(repository *github.Repository) (*git.Repository, *git.Worktree, error) {
	err := os.RemoveAll(".temp")
	if err != nil {
		fmt.Println("error deleting temp directory:", err)
	}

	gitRepo, err := git.PlainClone(".temp", false, &git.CloneOptions{
		URL:      *repository.CloneURL,
		Progress: os.Stdout,
	})

	workTree, err := gitRepo.Worktree()
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(" > repository cloned")
	return gitRepo, workTree, err
}

func checkoutBranch(gitRepo *git.Repository, workTree *git.Worktree) error {
	opts := &git.CheckoutOptions{
		Branch: "refs/heads/bucharest",
		Create: true,
	}
	err := workTree.Checkout(opts)
	if err != nil {
		fmt.Println("checkout error: ", err)
	}

	return err
}

func commitChanges(gitRepo *git.Repository, workTree *git.Worktree, user *github.User) error {
	workTree.AddGlob(".")
	//should go to log: status, _ := workTree.Status()
	commit, err := workTree.Commit("example go-git commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  user.GetName(),
			Email: user.GetEmail(),
			When:  time.Now(),
		},
	})

	_, err = gitRepo.CommitObject(commit)
	if err != nil {
		fmt.Println(err)
	}

	return err
}

func pushChanges(gitRepo *git.Repository, token *string) error {
	opts := &git.PushOptions{
		Auth: &http.BasicAuth{
			Username: " ",
			Password: *token,
		},
	}
	err := gitRepo.Push(opts)
	if err != nil {
		fmt.Println(err)
	}

	return err
}
