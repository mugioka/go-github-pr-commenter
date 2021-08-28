package commenter

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/google/go-github/v32/github"
)

// Commenter is the main commenter struct
type Commenter struct {
	ghConnector      *connector
	existingComments []*existingComment
	files            []*commitFileInfo
}

var (
	patchRegex     = regexp.MustCompile(`^@@.*\+(\d+),(\d+).+?@@`)
	commitRefRegex = regexp.MustCompile(".+ref=(.+)")
)

// NewCommenter creates a Commenter for updating PR with comments
func NewCommenter(token, owner, repo string, prNumber int) (*Commenter, error) {

	if len(token) == 0 {
		return nil, errors.New("the GITHUB_TOKEN has not been set")
	}

	ghConnector, err := createConnector(token, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	commitFileInfos, existingComments, err := ghConnector.getPRInfo()
	if err != nil {
		return nil, err
	}

	return &Commenter{
		ghConnector:      ghConnector,
		existingComments: existingComments,
		files:            commitFileInfos,
	}, nil
}

// WriteMultiLineComment writes a multiline review on a file in the github PR
func (c *Commenter) WriteMultiLineComment(file, comment string, startLine, endLine int) error {

	if !c.checkCommentRelevant(file, startLine) || !c.checkCommentRelevant(file, endLine) {
		return newCommentNotValidError(file, startLine)
	}

	if startLine == endLine {
		return c.WriteLineComment(file, comment, endLine)
	}

	info, err := c.getFileInfo(file, endLine)
	if err != nil {
		return err
	}

	prComment := buildComment(file, comment, endLine, *info)
	prComment.StartLine = &startLine
	return c.writeCommentIfRequired(prComment)
}

// WriteLineComment writes a single review line on a file of the github PR
func (c *Commenter) WriteLineComment(file, comment string, line int) error {

	if !c.checkCommentRelevant(file, line) {
		return newCommentNotValidError(file, line)
	}

	info, err := c.getFileInfo(file, line)
	if err != nil {
		return err
	}
	prComment := buildComment(file, comment, line, *info)
	return c.writeCommentIfRequired(prComment)
}

func (c *Commenter) WriteGeneralComment(comment string) error {

	issueComment := &github.IssueComment{
		Body: &comment,
	}
	return c.ghConnector.writeGeneralComment(issueComment)
}

func (c *Commenter) writeCommentIfRequired(prComment *github.PullRequestComment) error {

	var commentId *int64
	for _, existing := range c.existingComments {
		commentId = func(ec *existingComment) *int64 {
			if *ec.filename == *prComment.Path && *ec.comment == *prComment.Body {
				return ec.commentId
			}
			return nil
		}(existing)
	}

	if err := c.ghConnector.writeReviewComment(prComment, commentId); err != nil {
		return fmt.Errorf("write review comment: %w", err)
	}
	return nil
}

func (c *Commenter) checkCommentRelevant(filename string, line int) bool {

	for _, file := range c.files {
		if relevant := func(file *commitFileInfo) bool {
			if file.FileName == filename {
				if line >= file.hunkStart && line <= file.hunkEnd {
					return true
				}
			}
			return false
		}(file); relevant {
			return true
		}
	}
	return false
}

func (c *Commenter) getFileInfo(file string, line int) (*commitFileInfo, error) {

	for _, info := range c.files {
		if info.FileName == file {
			if line >= info.hunkStart && line <= info.hunkEnd {
				return info, nil
			}
		}
	}
	return nil, errors.New("file not found, shouldn't have got to here")
}

func buildComment(file, comment string, line int, info commitFileInfo) *github.PullRequestComment {

	return &github.PullRequestComment{
		Line:     &line,
		Path:     &file,
		CommitID: &info.sha,
		Body:     &comment,
		Position: info.calculatePosition(line),
	}
}
