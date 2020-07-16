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
	startTime := flag.String("time", "", "Select from which time to start from which we should start searching code for master/slave. It goes from that moment to older code. Default: from now.")
	flag.Parse()
	handleTokenFlag(token)

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	timeToSearch := handleTimeFlag(startTime)
	user := handleUserFlag(ctx, client, email)
	fmt.Printf("identity: %s / %s\n", user.GetName(), user.GetEmail())

	for {
		searchCodeHour(ctx, client, user, token, &timeToSearch)
		timeToSearch = timeToSearch.Add(-1 * time.Hour)
	}
}

//SearchCodeHour searches for code in given one-hour time window, it handles all the paginations.
func searchCodeHour(ctx context.Context, client *github.Client, user *github.User, token *string, searchTime *time.Time) {
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		resp, err := searchCodePageAndFix(ctx, client, opts, user, token, searchTime)
		if err != nil {
			fmt.Println(err)
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}
}

//SearchCodePageAndFix searches for one page of code search results and on every result it calls a fix.
func searchCodePageAndFix(ctx context.Context, client *github.Client, opts *github.SearchOptions, user *github.User, token *string, searchTime *time.Time) (*github.Response, error) {
	t := searchTime.Format(time.RFC3339)
	search := fmt.Sprintf("slave sort:date created:%v", t[:13])

	fmt.Printf("=====  %v  =====\n", t)

	results, resp, err := client.Search.Code(ctx, search, opts)
	handleAPILimit(resp)
	if err != nil {
		return resp, err
	}

	for _, result := range results.CodeResults {
		fmt.Println(result.GetRepository().GetFullName())
		//fixRepository(ctx, result.Repository, client, user, token)
	}

	return resp, nil
}

func handleUserFlag(ctx context.Context, client *github.Client, email *string) *github.User {
	user, resp, err := client.Users.Get(ctx, "")
	handleAPILimit(resp)
	if err != nil {
		fmt.Println("error getting the user info:", err)
		os.Exit(103)
	}

	if *email != "" {
		user.Email = email
		return user
	}

	if user.GetEmail() != "" {
		return user
	}

	emails, resp, err := client.Users.ListEmails(ctx, &github.ListOptions{})
	handleAPILimit(resp)
	if err != nil {
		fmt.Println("error getting the email for the user:", err)
		fmt.Println("if this persist, please provide the email as flag --email")
		os.Exit(104)
	}

	for _, email := range emails {
		if email.GetVisibility() == "private" {
			continue
		}

		user.Email = email.Email
		break
	}

	return user
}

func handleTimeFlag(startTime *string) time.Time {
	if *startTime == "" {
		timeToSearch := time.Now().UTC()
		timeToSearch = timeToSearch.Add(30 * time.Minute)
		timeToSearch = timeToSearch.Round(time.Hour)
		return timeToSearch
	}

	timeToSearch, err := time.Parse((time.RFC3339), *startTime)
	if err != nil {
		fmt.Println("error parsing the time flag:", err)
		os.Exit(101)
	}

	return timeToSearch
}

func handleTokenFlag(token *string) {
	if *token == "" {
		fmt.Println("Token flag is required.")
		os.Exit(100)
	}

	return
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
	fmt.Printf("\nREPO: %s\n", *repository.FullName)
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
	unslave.Unslave(workTree)
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
		Progress: nil,
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
	/* 	err := workTree.AddGlob(".")
	   	if err != nil {
	   		fmt.Println("addglob error:", err)
	   	} */

	//should go to log: status, _ := workTree.Status()
	commit, err := workTree.Commit("example go-git commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  user.GetName(),
			Email: user.GetEmail(),
			When:  time.Now(),
		},
	})

	if err != nil {
		fmt.Println("error creating the comit", err)
	}

	_, err = gitRepo.CommitObject(commit)
	if err != nil {
		fmt.Println("error comiting the object:", err)
	}

	return err
}

func pushChanges(gitRepo *git.Repository, token *string) error {
	opts := &git.PushOptions{
		Force: true,
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
