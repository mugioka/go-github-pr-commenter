package commenter

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

const (
	CommenterName = "github-action[bot]"
)

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

// create github connector and check if supplied pr number exists
func createConnector(token, owner, repo string, prNumber int) (*connector, error) {

	client := newGithubClient(token)
	if _, _, err := client.PullRequests.Get(context.Background(), owner, repo, prNumber); err != nil {
		return nil, newPRDoesNotExistError(owner, repo, prNumber)
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

func (c *connector) getPRInfo() ([]*CommitFileInfo, []*existingComment, error) {

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

func (c *connector) getCommitFileInfo() ([]*CommitFileInfo, error) {

	prFiles, err := c.getFilesForPr()
	if err != nil {
		return nil, err
	}

	var (
		errs            []string
		commitFileInfos []*CommitFileInfo
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

func getCommitInfo(file *github.CommitFile) (*CommitFileInfo, error) {

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

	return &CommitFileInfo{
		fileName:  *file.Filename,
		hunkStart: hunkStart,
		hunkEnd:   hunkStart + (hunkEnd - 1),
		sha:       sha,
	}, nil
}

func (c *connector) CreatePRReview(ctx context.Context, event string, body string, comments []*github.DraftReviewComment) error {
	review := &github.PullRequestReviewRequest{
		Body:     &body,
		Event:    &event,
		Comments: comments,
	}
	if _, _, err := c.prs.CreateReview(ctx, c.owner, c.repo, c.prNumber, review); err != nil {
		return err
	}
	return nil
}

func (c *connector) DeletePRReviewComment(ctx context.Context, commentID *int64) error {
	if _, err := c.prs.DeleteComment(ctx, c.owner, c.repo, *commentID); err != nil {
		return fmt.Errorf("delete existing comment %d: %w", *commentID, err)
	}
	return nil
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
		if CommenterName == *comment.User.Login {
			existingComments = append(existingComments, &existingComment{
				filename:  comment.Path,
				comment:   comment.Body,
				commentId: comment.ID,
			})
		}
	}
	return existingComments, nil
}
