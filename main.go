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
		//searchCodeHour(ctx, client, user, token, &timeToSearch)
		searchCodeDay(ctx, client, user, token, &timeToSearch)
		timeToSearch = timeToSearch.Add(-24 * time.Hour)
	}
}

//SearchCodeDay searches for code in given one-hour time window, it handles all the paginations.
func searchCodeDay(ctx context.Context, client *github.Client, user *github.User, token *string, searchTime *time.Time) {
	fmt.Printf("=====  %v  =====\n", searchTime.Format(time.RFC3339))
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		t := searchTime.Format(time.RFC3339)[:10]
		resp := searchCodePageAndFix(ctx, client, opts, user, token, t)
		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}
}

//SearchCodeHour searches for code in given one-hour time window, it handles all the paginations.
func searchCodeHour(ctx context.Context, client *github.Client, user *github.User, token *string, searchTime *time.Time) {
	fmt.Printf("=====  %v  =====\n", searchTime.Format(time.RFC3339))
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		t := searchTime.Format(time.RFC3339)[:13]
		resp := searchCodePageAndFix(ctx, client, opts, user, token, t)
		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}
}

//SearchCodePageAndFix searches for one page of code search results and on every result it calls a fix.
func searchCodePageAndFix(ctx context.Context, client *github.Client, opts *github.SearchOptions, user *github.User, token *string, searchTime string) *github.Response {
	search := fmt.Sprintf("slave sort:date created:%v language:python language:go", searchTime)

	var results *github.CodeSearchResult
	var resp *github.Response
	for i := 1; i <= 5; i++ {
		var err error
		results, resp, err = client.Search.Code(ctx, search, opts)
		handleAPILimit(resp)
		if err != nil {
			fmt.Printf("error searching for code '%v'\ntrying it again in 30 sec...\n", err)
			time.Sleep(30 * time.Second)
			continue
		}
		break
	}

	for _, result := range results.CodeResults {
		fixRepository(ctx, result.Repository, client, user, token)
	}

	return resp
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
	//maybe this should be in global config
	title := unslave.GeneratePRTitle()
	description := unslave.GeneratePRDescription()
	gitHead, err := gitRepo.Head()
	if err != nil {
		fmt.Printf("error getting git HEAD reference: %v\n", err)
	}
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

	var resp *github.Response
	for i := 1; i <= 5; i++ {
		_, resp, err = client.PullRequests.Create(ctx, *repository.Owner.Login, repository.GetName(), newPR)
		handleAPILimit(resp)
		if err != nil {
			fmt.Printf("error creating the PR: %v, trying it again in 30 seconds...\n", err)
			time.Sleep(30 * time.Second)
			continue
		}

		fmt.Print(" > PR created!!!")
		return nil
	}

	return err
}

func handleAPILimit(response *github.Response) {
	if response.Rate.Remaining < 5 {
		fmt.Println(" ---- API limit is near, waiting for it to reset:", response.Rate)
		length := time.Until(response.Rate.Reset.Time) + time.Duration(time.Second*5)
		time.Sleep(length)
	}
}

func fixRepository(ctx context.Context, repository *github.Repository, client *github.Client, user *github.User, token *string) error {
	fmt.Printf("\nREPO: %s\n", *repository.FullName)
	if fixExists(ctx, repository, client) {
		return nil
	}

	fork, err := forkRepo(ctx, repository, client)
	if err != nil {
		return err
	}

	if *fork.Size > 200000 {
		fmt.Printf(" > repository too big (%vkb), skipping...\n", *fork.Size)
		return nil
	}

	gitRepo, workTree, err := cloneRepo(fork)
	if err != nil {
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

	var pullRequests []*github.PullRequest
	var resp *github.Response
	for i := 1; i <= 5; i++ {
		var err error
		pullRequests, resp, err = client.PullRequests.List(ctx, *repository.Owner.Login, *repository.Name, opts)
		handleAPILimit(resp)
		if err != nil {
			fmt.Printf("error listing PRs %v, trying it again in 30 seconds", err)
			time.Sleep(30 * time.Second)
			continue
		}
		break
	}

	for _, pullRequest := range pullRequests {
		//searches in additional text of the PR
		if strings.Contains(*pullRequest.Body, "which can be associated to slavery") {
			fmt.Println(" > fix exists!")
			return true
		}
	}

	return false
}

func forkRepo(ctx context.Context, repository *github.Repository, client *github.Client) (*github.Repository, error) {
	opts := &github.RepositoryCreateForkOptions{}
	owner := *repository.Owner.Login
	name := repository.GetName()

	var repo *github.Repository
	var resp *github.Response
	var err error
	for i := 1; i <= 5; i++ {
		var err error
		repo, resp, err = client.Repositories.CreateFork(ctx, owner, name, opts)
		handleAPILimit(resp)
		if err.Error() == "job scheduled on GitHub side; try again later" {
			fmt.Println(" > repo forked")
			return repo, nil
		}
		if err != nil {
			fmt.Printf("error forking repository: %v, trying it again in 30 seconds...\n", err)
			time.Sleep(30 * time.Second)
			continue
		}
		fmt.Println(" > repo forked")
		return repo, nil
	}

	return repo, err
}

func cloneRepo(repository *github.Repository) (*git.Repository, *git.Worktree, error) {
	fmt.Println(" > cloning repo")
	err := os.RemoveAll(".temp")
	if err != nil {
		fmt.Println("error deleting temp directory:", err)
	}

	opts := &git.CloneOptions{
		URL:      *repository.CloneURL,
		Progress: os.Stdout,
	}
	var gitRepo *git.Repository
	for i := 1; i <= 5; i++ {
		gitRepo, err = git.PlainClone(".temp", false, opts)
		if err != nil {
			fmt.Printf("error cloning the repo: %v, trying it again in 5 seconds...\n", err)
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}

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
	//should go to log: status, _ := workTree.Status()
	commit, err := workTree.Commit(unslave.GeneratePRTitle(), &git.CommitOptions{
		Author: &object.Signature{
			Name:  user.GetName(),
			Email: user.GetEmail(),
			When:  time.Now(),
		},
	})

	if err != nil {
		fmt.Println("error creating the comit", err)
		return err
	}

	_, err = gitRepo.CommitObject(commit)
	if err != nil {
		fmt.Println("error comiting the object:", err)
		return err
	}

	fmt.Println(" > commited")
	return nil
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
		return err
	}

	fmt.Println(" > changes pushed")
	return nil
}
