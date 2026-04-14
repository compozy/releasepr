package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/compozy/releasepr/internal/repository"
	"github.com/compozy/releasepr/internal/usecase"
	"github.com/spf13/cobra"
)

// NewAddNoteCmd creates the add-note command.
func NewAddNoteCmd(fsRepo repository.FileSystemRepository) *cobra.Command {
	var (
		title    string
		noteType string
		body     string
	)
	cmd := &cobra.Command{
		Use:   "add-note",
		Short: "Create a custom release note entry",
		RunE: func(cmd *cobra.Command, _ []string) error {
			uc := &usecase.CreateReleaseNoteUseCase{
				FSRepo: fsRepo,
			}
			path, err := uc.Execute(cmd.Context(), usecase.CreateReleaseNoteInput{
				Title: title,
				Type:  noteType,
				Body:  body,
			})
			if err != nil {
				return err
			}
			if body == "" {
				opened, openErr := openReleaseNoteEditor(cmd.Context(), path)
				if openErr != nil {
					return openErr
				}
				if !opened {
					cmd.Printf("Created %s\n", path)
				}
				return nil
			}
			cmd.Printf("Created %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Release note title")
	cmd.Flags().StringVar(&noteType, "type", "", "Release note type: feature, fix, breaking, highlight")
	cmd.Flags().StringVar(&body, "body", "", "Inline release note body. Skips opening $EDITOR")
	if err := cmd.MarkFlagRequired("title"); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired("type"); err != nil {
		panic(err)
	}
	return cmd
}

func openReleaseNoteEditor(ctx context.Context, path string) (bool, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return false, nil
	}
	cmd := exec.CommandContext(ctx, "sh", "-lc", "exec $EDITOR \"$1\"", "sh", path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("failed to open editor: %w", err)
	}
	return true, nil
}
