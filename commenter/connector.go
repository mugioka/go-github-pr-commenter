package commenter

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

const githubAbuseErrorRetries = 6

type connector struct {
	prs      *github.PullRequestsService
	comments *github.IssuesService
	owner    string
	repo     string
	prNumber int
}

type existingComment struct {
	filename  *string
	comment   *string
	commentId *int64
}

type commentFn func() (*github.Response, error)

// create github connector and check if supplied pr number exists
func createConnector(token, owner, repo string, prNumber int) (*connector, error) {

	client := newGithubClient(token)
	if _, _, err := client.PullRequests.Get(context.Background(), owner, repo, prNumber); err != nil {
		return nil, newPrDoesNotExistError(owner, repo, prNumber)
	}

	return &connector{
		prs:      client.PullRequests,
		comments: client.Issues,
		owner:    owner,
		repo:     repo,
		prNumber: prNumber,
	}, nil
}

func newGithubClient(token string) *github.Client {

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)
}

func (c *connector) getPRInfo() ([]*commitFileInfo, []*existingComment, error) {

	commitFileInfos, err := c.getCommitFileInfo()
	if err != nil {
		return nil, nil, err
	}

	existingComments, err := c.getExistingComments()
	if err != nil {
		return nil, nil, err
	}
	return commitFileInfos, existingComments, nil
}

func (c *connector) getCommitFileInfo() ([]*commitFileInfo, error) {

	prFiles, err := c.getFilesForPr()
	if err != nil {
		return nil, err
	}

	var (
		errs            []string
		commitFileInfos []*commitFileInfo
	)

	for _, file := range prFiles {
		info, err := getCommitInfo(file)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		commitFileInfos = append(commitFileInfos, info)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("there were errors processing the PR files.\n%s", strings.Join(errs, "\n"))
	}
	return commitFileInfos, nil
}

func getCommitInfo(file *github.CommitFile) (*commitFileInfo, error) {

	groups := patchRegex.FindAllStringSubmatch(file.GetPatch(), -1)
	var hunkStart, hunkEnd int
	if len(groups) < 1 {
		if file.GetChanges() >= 1 {
			hunkStart, hunkEnd = 1, 1
		} else {
			return nil, errors.New("the patch details could not be resolved")
		}
	} else {
		hunkStart, _ = strconv.Atoi(groups[0][1])
		hunkEnd, _ = strconv.Atoi(groups[0][2])
	}

	shaGroups := commitRefRegex.FindAllStringSubmatch(file.GetContentsURL(), -1)
	if len(shaGroups) < 1 {
		return nil, errors.New("the sha details could not be resolved")
	}
	sha := shaGroups[0][1]

	return &commitFileInfo{
		FileName:  *file.Filename,
		hunkStart: hunkStart,
		hunkEnd:   hunkStart + (hunkEnd - 1),
		sha:       sha,
	}, nil
}

func (c *connector) writeReviewComment(block *github.PullRequestComment, commentId *int64) error {

	ctx := context.Background()
	if commentId != nil {
		if _, err := c.prs.DeleteComment(ctx, c.owner, c.repo, *commentId); err != nil {
			return fmt.Errorf("delete existing comment %d: %w", *commentId, err)
		}
	}

	writeReviewCommentFn := func() (*github.Response, error) {
		_, resp, err := c.prs.CreateComment(ctx, c.owner, c.repo, c.prNumber, block)
		return resp, err
	}
	return writeCommentWithRetries(c.owner, c.repo, c.prNumber, writeReviewCommentFn)
}

func (c *connector) writeGeneralComment(comment *github.IssueComment) error {

	ctx := context.Background()
	writeReviewCommentFn := func() (*github.Response, error) {
		_, resp, err := c.comments.CreateComment(ctx, c.owner, c.repo, c.prNumber, comment)
		return resp, err
	}
	return writeCommentWithRetries(c.owner, c.repo, c.prNumber, writeReviewCommentFn)
}

func writeCommentWithRetries(owner, repo string, prNumber int, commentFn commentFn) error {

	var abuseError AbuseRateLimitError
	for i := 0; i < githubAbuseErrorRetries; i++ {

		retrySeconds := i * i
		time.Sleep(time.Second * time.Duration(retrySeconds))

		if resp, err := commentFn(); err != nil {
			if resp != nil && resp.StatusCode == 422 {
				abuseError = newAbuseRateLimitError(owner, repo, prNumber, retrySeconds)
				continue
			}
			return fmt.Errorf("write comment: %v", err)
		}
		return nil
	}
	return abuseError
}

func (c *connector) getFilesForPr() ([]*github.CommitFile, error) {

	files, _, err := c.prs.ListFiles(context.Background(), c.owner, c.repo, c.prNumber, nil)
	if err != nil {
		return nil, err
	}

	var commitFiles []*github.CommitFile
	for _, file := range files {
		if *file.Status != "deleted" {
			commitFiles = append(commitFiles, file)
		}
	}
	return commitFiles, nil
}

func (c *connector) getExistingComments() ([]*existingComment, error) {

	ctx := context.Background()
	comments, _, err := c.prs.ListComments(ctx, c.owner, c.repo, c.prNumber, &github.PullRequestListCommentsOptions{})
	if err != nil {
		return nil, err
	}

	var existingComments []*existingComment
	for _, comment := range comments {
		existingComments = append(existingComments, &existingComment{
			filename:  comment.Path,
			comment:   comment.Body,
			commentId: comment.ID,
		})
	}
	return existingComments, nil
}
