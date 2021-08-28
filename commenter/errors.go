package commenter

import "fmt"

// CommentAlreadyWrittenError returned when the error can't be written as it already exists
type CommentAlreadyWrittenError struct {
	filepath string
	comment  string
}

// CommentNotValidError returned when the comment is for a file or line not in the pr
type CommentNotValidError struct {
	filepath string
	lineNo   int
}

// PRDoesNotExistError returned when the PR can't be found, either as 401 or not existing
type PRDoesNotExistError struct {
	owner    string
	repo     string
	prNumber int
}

// AbuseRateLimitError return when the GitHub abuse rate limit is hit
type AbuseRateLimitError struct {
	owner            string
	repo             string
	prNumber         int
	BackoffInSeconds int
}

func newPRDoesNotExistError(owner, repo string, prNumber int) PRDoesNotExistError {
	return PRDoesNotExistError{
		owner:    owner,
		repo:     repo,
		prNumber: prNumber,
	}
}

func (e CommentAlreadyWrittenError) Error() string {
	return fmt.Sprintf("The file [%s] already has the comment written [%s]", e.filepath, e.comment)
}

func (e CommentNotValidError) Error() string {
	return fmt.Sprintf("There is nothing to comment on at line [%d] in file [%s]", e.lineNo, e.filepath)
}

func (e PRDoesNotExistError) Error() string {
	return fmt.Sprintf("PR number [%d] not found for %s/%s", e.prNumber, e.owner, e.repo)
}

func (e AbuseRateLimitError) Error() string {
	return fmt.Sprintf("Abuse limit reached on PR [%d] not found for %s/%s", e.prNumber, e.owner, e.repo)
}
