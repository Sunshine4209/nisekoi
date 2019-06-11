package calc

import (
	"context"
	"fmt"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	"net/http"
	"projects/cal"
	"strings"
	"sync"
	"time"
)

type Cmd struct {
	Owner       string
	Repository  string
	Username    string
	AccessToken string
	Debug       bool
}

type Repository struct {
	Owner string
	Name  string
}

type Result struct {
	PullRequests []*github.PullRequest
	Err          error
}

func (cmd Cmd) Run() error {
	calendar := cal.NewCalendar()

	ctx := context.Background()
	var tc *http.Client
	if len(cmd.AccessToken) != 0 {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: cmd.AccessToken},
		)
		tc = oauth2.NewClient(ctx, ts)
	}
	client := github.NewClient(tc)

	var repos []Repository
	if len(cmd.Repository) != 0 {
		repos = append(repos, Repository{Owner: cmd.Owner, Name: cmd.Repository})
	} else {
		response, err := getRepositories(ctx, client, cmd.Owner)
		if err != nil {
			return err
		}
		repos = append(repos, response...)
	}

	c := make(chan Result, len(repos))
	var wg sync.WaitGroup
	for _, repo := range repos {
		wg.Add(1)
		go func(val Repository) {
			if cmd.Debug {
				fmt.Printf("Executing goroutine for value: %s\n", val.Name)
			}
			pullRequests, err := getPullRequests(ctx, client, val)
			c <- Result{PullRequests: pullRequests, Err: err}
			wg.Done()
		}(repo)
	}
	wg.Wait()
	close(c)

	timeAccumulator := float64(0)
	prAccumulator := int64(0)
	userPrAccumulator := int64(0)

	var cmdErr error
	for result := range c {
		pullRequests, err := result.PullRequests, result.Err
		if err != nil {
			cmdErr = err
			break
		}
		for _, pullRequest := range pullRequests {
			prAccumulator++
			if len(cmd.Username) != 0 {
				if !strings.EqualFold(pullRequest.GetUser().GetLogin(), cmd.Username) {
					continue
				}
				userPrAccumulator++
			}

			createWorkday := pullRequest.GetCreatedAt().Weekday();
			who := pullRequest.GetUser().GetLogin()
			state := pullRequest.GetState()

			if pullRequest.GetMergedAt().IsZero() {
				if cmd.Debug {
					fmt.Printf("%v PR: %s\nCreated by  : %v\n", state, pullRequest.GetTitle(), who)
					fmt.Printf("Created on a : %v %v\n\n", createWorkday, pullRequest.GetCreatedAt())
				}
			} else {

				mergeWorkday := pullRequest.GetMergedAt().Weekday();
				whoMerge := pullRequest.GetMergedBy().GetLogin();
				workdays := calendar.CountWorkdays(pullRequest.GetCreatedAt(), pullRequest.GetMergedAt())

				delta := float64(0)
				if float64(workdays * 24) < pullRequest.GetMergedAt().Sub(pullRequest.GetCreatedAt()).Hours() {
					delta = float64(workdays * 24)
				} else {
					delta = pullRequest.GetMergedAt().Sub(pullRequest.GetCreatedAt()).Hours()
				}
				timeAccumulator += delta

				if cmd.Debug {
					fmt.Printf("%v PR: %s\nCreated by: %v\nCreated on a: %v\nMerged on a: %v\nMerged by  : %v\n", state, pullRequest.GetTitle(), who, createWorkday, mergeWorkday, whoMerge)
					fmt.Printf("Created on a : %v\nMerged at:%v\nDelta in hours: %f \ntotal woking days PR was open %d \n\n", pullRequest.GetCreatedAt(), pullRequest.GetMergedAt(), delta, workdays)
				}
			}
		}
	}

	userLanded := ""
	average := timeAccumulator / float64(prAccumulator)
	if len(cmd.Username) != 0 {
		userLanded = fmt.Sprintf("%d out of ", userPrAccumulator)
		average = timeAccumulator / float64(userPrAccumulator)
	}
	fmt.Printf("Average landing PR time is: %.2f hours, for a total of %s%d landed PRs\n\n", average, userLanded, prAccumulator)

	return cmdErr
}

func getPullRequests(ctx context.Context, client *github.Client, repository Repository) ([]*github.PullRequest, error) {
	opt := &github.PullRequestListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
		State:       "all",
	}

	var apiResponse []*github.PullRequest
	for {
		pullRequests, resp, err := client.PullRequests.List(ctx, repository.Owner, repository.Name, opt)
		if err != nil {
			return nil, err
		}

		for _, pullRequest := range pullRequests {
			if pullRequest.GetCreatedAt().After(time.Date(2019, time.January, 1, 0, 0, 0, 0, time.UTC)) {
				apiResponse = append(apiResponse, pullRequest)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return apiResponse, nil
}

func getRepositories(ctx context.Context, client *github.Client, org string) ([]Repository, error) {
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var apiResponse []*github.Repository
	for {
		repos, resp, err := client.Repositories.ListByOrg(ctx, org, opt)
		if err != nil {
			return nil, err
		}
		apiResponse = append(apiResponse, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	var repositories []Repository
	for _, item := range apiResponse {
		repositories = append(repositories, Repository{Owner: item.GetOwner().GetLogin(), Name: item.GetName()})
	}

	return repositories, nil
}
