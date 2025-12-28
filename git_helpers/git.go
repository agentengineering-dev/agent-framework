package git_helpers

import (
	"fmt"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"os"
	"time"
)

var repo *git.Repository

func Init() {
	var err error
	var r *git.Repository
	r, _, err = getOrInitRepo(".")
	if err != nil {
		panic(err)
	}
	repo = r
}

func getOrInitRepo(path string) (*git.Repository, bool, error) {
	// ensure directory exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return nil, false, err
		}
	}

	// Try to open existing repo
	repo, err := git.PlainOpen(path)
	if err == nil {
		return repo, false, nil
	}

	// If not exists, initialize
	if err == git.ErrRepositoryNotExists {
		repo, err := git.PlainInit(path, false)
		if err != nil {
			return nil, false, err
		}
		return repo, true, nil
	}

	// Other errors bubble up
	return nil, false, err
}

func CreateBranch(branchName string) error {
	if repo == nil {
		return fmt.Errorf("git_helpers.Init() was not called")
	}

	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	return w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	})
}

func AddAllAndCommit(message, authorName, authorEmail string) error {
	// Get worktree
	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	// Stage all changes
	// equivalent to: git add .
	err = w.AddWithOptions(&git.AddOptions{
		All: true,
	})
	if err != nil {
		return err
	}

	// Commit
	_, err = w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})

	return err
}
