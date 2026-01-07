package git_helpers

import (
	"fmt"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"os"
	"strings"
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
		fmt.Println("Agent: Found and opened git repository")
		return repo, false, nil
	}

	// If not exists, initialize
	if err == git.ErrRepositoryNotExists {
		repo, err := git.PlainInit(path, false)
		if err != nil {
			return nil, false, err
		}
		fmt.Println("Agent: Inited and opened git repository")

		// create main branch
		handRef := plumbing.NewSymbolicReference(
			plumbing.HEAD,
			plumbing.NewBranchReferenceName("main"),
		)
		if err := repo.Storer.SetReference(handRef); err != nil {
			return nil, false, err
		}

		wt, err := repo.Worktree()
		if err != nil {
			return nil, false, err
		}

		err = os.WriteFile(".gitkeep", []byte(""), 0o644)
		if err != nil {
			return nil, false, err
		}

		_, err = wt.Add(".gitkeep")
		if err != nil {
			return nil, false, err
		}
		_, err = wt.Commit("initial commit", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "agent-framework",
				Email: "agent-framework@agentengineering.dev",
				When:  time.Now(),
			},
		})
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

	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	})

	if err == nil {
		fmt.Println("Agent: Created branch: ", branchName)
		return nil
	}

	if strings.Contains(err.Error(), "already exists") {
		suffix := time.Now().Format("20060102150405")
		newName := fmt.Sprintf("%s-%s", branchName, suffix)

		fmt.Println("Agent: Branch exists, Created new branch: ", newName)
		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName(branchName),
			Create: true,
		})
		return nil

	}
	return err
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
