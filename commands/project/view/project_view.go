package view

import (
	"encoding/base64"
	"fmt"
	"github.com/MakeNowJust/heredoc"
	"github.com/profclems/glab/api"
	"github.com/profclems/glab/commands/cmdutils"
	"github.com/profclems/glab/pkg/git"
	"github.com/profclems/glab/pkg/iostreams"
	"github.com/profclems/glab/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/xanzy/go-gitlab"
	"strings"
)

type ViewOptions struct {
	ProjectID    string
	ApiClient    *gitlab.Client
	Web          bool
	Branch       string
	Browser      string
	GlamourStyle string

	IO *iostreams.IOStreams
}

func NewCmdView(f *cmdutils.Factory) *cobra.Command {
	opts := ViewOptions{
		IO: f.IO,
	}

	var projectViewCmd = &cobra.Command{
		Use:   "view [repository] [flags]",
		Short: "View a project/repository",
		Long:  `Display the description and README of a project or open it in the browser.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := f.BaseRepo()
			if err != nil {
				return err
			}

			if opts.ProjectID == "" {
				if len(args) == 1 {
					opts.ProjectID = args[0]
				} else {
					opts.ProjectID = repo.FullName()
				}
			}

			if opts.Branch == "" {
				opts.Branch, err = git.CurrentBranch()
				if err != nil {
					return err
				}
			}

			cfg, err := f.Config()
			if err != nil {
				return err
			}

			browser, _ := cfg.Get(repo.RepoHost(), "browser")
			opts.Browser = browser

			opts.GlamourStyle, _ = cfg.Get(repo.RepoHost(), "glamour_style")

			apiClient, err := f.HttpClient()
			if err != nil {
				return err
			}

			opts.ApiClient = apiClient

			return runViewProject(&opts)
		},
		Example: heredoc.Doc(`
			# view project information for the current directory
			$ glab repo view

			# view project information of specified name
			$ glab repo view my-project
		`),
	}

	projectViewCmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open a project in the browser")
	projectViewCmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "View a specific branch of the repository")

	return projectViewCmd
}

func runViewProject(opts *ViewOptions) error {
	repoPath := opts.ProjectID

	if !strings.Contains(repoPath, "/") {
		currentUser, err := api.CurrentUser(opts.ApiClient)

		if err != nil {
			fmt.Fprintf(opts.IO.StdErr, "Failed to retrieve your current user: %s", err)

			return err
		}

		repoPath = currentUser.Username + "/" + repoPath
	}

	project, err := api.GetProject(opts.ApiClient, repoPath)
	if err != nil {
		fmt.Fprintf(opts.IO.StdErr, "Failed to access project API: %s", err)

		return err
	}

	readmeFile := getReadmeFile(opts, project)

	if opts.Web {
		openURL := generateProjectURL(project, opts.Branch)

		if opts.IO.IsaTTY {
			fmt.Fprintf(opts.IO.StdOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}

		return utils.OpenInBrowser(openURL, opts.Browser)
	} else {
		if opts.IO.IsaTTY {
			printProjectContentTTY(opts, project, readmeFile)
		} else {
			printProjectContentRaw(opts, project, readmeFile)
		}
	}

	return nil
}

func getReadmeFile(opts *ViewOptions, project *gitlab.Project) *gitlab.File {
	if project.ReadmeURL == "" {
		return nil
	}

	readmePath := strings.Replace(project.ReadmeURL, project.WebURL+"/-/blob/", "", 1)
	readmePathComponents := strings.Split(readmePath, "/")
	readmeRef := readmePathComponents[0]
	readmeFileName := readmePathComponents[1]
	readmeFile, err := api.GetFile(opts.ApiClient, project.PathWithNamespace, readmeFileName, readmeRef)

	if err != nil {
		fmt.Fprintf(opts.IO.StdErr, "Failed to retrieve README file: %s", err)

		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(readmeFile.Content)
	if err != nil {
		fmt.Fprintf(opts.IO.StdErr, "Failed to decode README file: %s", err)

		return nil
	}

	readmeFile.Content = string(decoded)

	return readmeFile
}

func generateProjectURL(project *gitlab.Project, branch string) string {
	if project.DefaultBranch != branch {
		return project.WebURL + "/-/tree/" + branch
	}

	return project.WebURL
}

func printProjectContentTTY(opts *ViewOptions, project *gitlab.Project, readme *gitlab.File) {
	var description string
	var readmeContent string
	var err error

	fullName := project.NameWithNamespace
	if project.Description != "" {
		description, err = utils.RenderMarkdownWithoutIndentations(project.Description, opts.GlamourStyle)

		if err != nil {
			description = project.Description
		}
	} else {
		description = "(No description provided)"
	}

	if readme != nil {
		readmeContent, err = utils.RenderMarkdown(readme.Content, opts.GlamourStyle)

		if err != nil {
			readmeContent = readme.Content
		}
	}

	c := opts.IO.Color()
	// Header
	fmt.Fprint(opts.IO.StdOut, c.Bold(fullName))
	fmt.Fprint(opts.IO.StdOut, c.Gray(description))

	if readme != nil {
		fmt.Fprint(opts.IO.StdOut, readmeContent)
	} else {
		fmt.Fprint(opts.IO.StdOut, c.Gray("This repository does not have a README file"))
	}

	fmt.Fprintln(opts.IO.StdOut)
	fmt.Fprintf(opts.IO.StdOut, c.Gray("View this project on GitLab: %s\n"), project.WebURL)
}

func printProjectContentRaw(opts *ViewOptions, project *gitlab.Project, readme *gitlab.File) {
	fullName := project.NameWithNamespace
	description := project.Description

	fmt.Fprintf(opts.IO.StdOut, "name:\t%s\n", fullName)
	fmt.Fprintf(opts.IO.StdOut, "description:\t%s\n", description)

	if readme != nil {
		fmt.Fprintln(opts.IO.StdOut, "---")
		fmt.Fprintf(opts.IO.StdOut, readme.Content)
		fmt.Fprintln(opts.IO.StdOut)
	}
}
