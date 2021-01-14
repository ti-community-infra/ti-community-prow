package autoresponder

import (
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/ti-community-infra/tichi/internal/pkg/externalplugins"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestAutoRespondIssueAndReviewComment(t *testing.T) {
	var testcases = []struct {
		name                string
		body                string
		responds            []externalplugins.AutoRespond
		shouldComment       bool
		expectCommentNumber int
	}{
		{
			name: "non-matching comment",
			body: "uh oh",
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment: false,
		},
		{
			name: "matching comment",
			body: "/merge",
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 1,
		},
		{
			name: "matching comment with trailing space",
			body: "/merge \r",
			responds: []externalplugins.AutoRespond{
				{
					Regex:   "(?mi)^/merge\\s*$",
					Message: "Got a merge command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 1,
		},
		{
			name: "matching comment with multiple auto responds",
			body: `/merge

                           /test`,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   "(?mi)^/merge\\s*$",
					Message: "Got a merge command.",
				},
				{
					Regex:   `/test`,
					Message: "Got a test command.",
				},
				{
					Regex:   `/foo`,
					Message: "Got a foo command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 2,
		},
	}

	SHA := "0bd3ed50c88cd53a09316bf7a298f900e9371652"

	for _, testcase := range testcases {
		tc := testcase
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Running scenario %s", tc.name)
			// Test issue comment.
			{
				fc := &fakegithub.FakeClient{
					IssueComments: make(map[int][]github.IssueComment),
				}

				e := &github.IssueCommentEvent{
					Action: github.IssueCommentActionCreated,
					Issue: github.Issue{
						User:   github.User{Login: "author"},
						Number: 5,
						State:  "open",
						PullRequest: &struct {
						}{},
					},
					Comment: github.IssueComment{
						Body:    tc.body,
						User:    github.User{Login: "user"},
						HTMLURL: "<url>",
					},
					Repo: github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				}

				cfg := &externalplugins.Configuration{}
				cfg.TiCommunityAutoresponder = []externalplugins.TiCommunityAutoresponder{
					{
						Repos:        []string{"org/repo"},
						AutoResponds: tc.responds,
					},
				}

				if err := HandleIssueCommentEvent(fc, e, cfg, logrus.WithField("plugin", PluginName)); err != nil {
					t.Errorf("didn't expect error from %s: %v", PluginName, err)
				}

				if tc.shouldComment && len(fc.IssueComments[5]) != tc.expectCommentNumber {
					t.Errorf("comments number mismatch: got %v, want %v", len(fc.IssueComments[5]), tc.expectCommentNumber)
				}
			}

			// Test pull request review comment.
			{
				fc := &fakegithub.FakeClient{
					IssueComments: make(map[int][]github.IssueComment),
					PullRequests: map[int]*github.PullRequest{
						5: {
							Base: github.PullRequestBranch{
								Ref: "master",
							},
							Head: github.PullRequestBranch{
								SHA: SHA,
							},
							User:   github.User{Login: "author"},
							Number: 5,
							State:  "open",
						},
					},
				}

				e := &github.ReviewCommentEvent{
					Action: github.ReviewCommentActionCreated,
					Comment: github.ReviewComment{
						Body:    tc.body,
						User:    github.User{Login: "user"},
						HTMLURL: "<url>",
					},
					Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
					PullRequest: *fc.PullRequests[5],
				}

				cfg := &externalplugins.Configuration{}
				cfg.TiCommunityAutoresponder = []externalplugins.TiCommunityAutoresponder{
					{
						Repos:        []string{"org/repo"},
						AutoResponds: tc.responds,
					},
				}

				if err := HandlePullReviewCommentEvent(fc, e, cfg, logrus.WithField("plugin", PluginName)); err != nil {
					t.Errorf("didn't expect error from %s: %v", PluginName, err)
				}

				if tc.shouldComment && len(fc.IssueComments[5]) != tc.expectCommentNumber {
					t.Errorf("comments number mismatch: got %v, want %v", len(fc.IssueComments[5]), tc.expectCommentNumber)
				}
			}
		})
	}
}

func TestAutoRespondPullRequestReview(t *testing.T) {
	var testcases = []struct {
		name                string
		body                string
		action              github.ReviewEventAction
		responds            []externalplugins.AutoRespond
		shouldComment       bool
		expectCommentNumber int
	}{
		{
			name:   "non-matching comment",
			body:   "uh oh",
			action: github.ReviewActionSubmitted,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment: false,
		},
		{
			name:   "matching comment",
			body:   "/merge",
			action: github.ReviewActionSubmitted,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 1,
		},
		{
			name:   "matching comment with trailing space",
			body:   "/merge \r",
			action: github.ReviewActionSubmitted,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   "(?mi)^/merge\\s*$",
					Message: "Got a merge command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 1,
		},
		{
			name: "matching comment with multiple auto responds",
			body: `/merge

                           /test`,
			action: github.ReviewActionSubmitted,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   "(?mi)^/merge\\s*$",
					Message: "Got a merge command.",
				},
				{
					Regex:   `/test`,
					Message: "Got a test command.",
				},
				{
					Regex:   `/foo`,
					Message: "Got a foo command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 2,
		},
		{
			name:   "edited action",
			body:   "/merge",
			action: github.ReviewActionEdited,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment: false,
		},
		{
			name:   "dismissed action",
			body:   "/merge",
			action: github.ReviewActionDismissed,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment: false,
		},
	}

	SHA := "0bd3ed50c88cd53a09316bf7a298f900e9371652"

	for _, testcase := range testcases {
		tc := testcase
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakegithub.FakeClient{
				IssueComments: make(map[int][]github.IssueComment),
				PullRequests: map[int]*github.PullRequest{
					5: {
						Base: github.PullRequestBranch{
							Ref: "master",
						},
						Head: github.PullRequestBranch{
							SHA: SHA,
						},
						User:   github.User{Login: "author"},
						Number: 5,
						State:  "open",
					},
				},
			}

			e := &github.ReviewEvent{
				Action:      tc.action,
				Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				PullRequest: *fc.PullRequests[5],
				Review: github.Review{
					Body:    tc.body,
					User:    github.User{Login: "user"},
					HTMLURL: "<url>",
				},
			}

			cfg := &externalplugins.Configuration{}
			cfg.TiCommunityAutoresponder = []externalplugins.TiCommunityAutoresponder{
				{
					Repos:        []string{"org/repo"},
					AutoResponds: tc.responds,
				},
			}

			if err := HandlePullReviewEvent(fc, e, cfg, logrus.WithField("plugin", PluginName)); err != nil {
				t.Errorf("didn't expect error from %s: %v", PluginName, err)
			}

			if tc.shouldComment && len(fc.IssueComments[5]) != tc.expectCommentNumber {
				t.Errorf("comments number mismatch: got %v, want %v", len(fc.IssueComments[5]), tc.expectCommentNumber)
			}
		})
	}
}

func TestAutoRespondPullRequest(t *testing.T) {
	var testcases = []struct {
		name                string
		body                string
		action              github.PullRequestEventAction
		responds            []externalplugins.AutoRespond
		shouldComment       bool
		expectCommentNumber int
	}{
		{
			name:   "non-matching comment",
			body:   "uh oh",
			action: github.PullRequestActionOpened,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment: false,
		},
		{
			name:   "matching comment",
			body:   "/merge",
			action: github.PullRequestActionOpened,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 1,
		},
		{
			name:   "matching comment with trailing space",
			body:   "/merge \r",
			action: github.PullRequestActionOpened,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   "(?mi)^/merge\\s*$",
					Message: "Got a merge command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 1,
		},
		{
			name: "matching comment with multiple auto responds",
			body: `/merge
		
                          /test`,
			action: github.PullRequestActionOpened,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   "(?mi)^/merge\\s*$",
					Message: "Got a merge command.",
				},
				{
					Regex:   `/test`,
					Message: "Got a test command.",
				},
				{
					Regex:   `/foo`,
					Message: "Got a foo command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 2,
		},
		{
			name:   "edited action",
			body:   "/merge",
			action: github.PullRequestActionEdited,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 1,
		},
		{
			name:   "labeled action",
			body:   "/merge",
			action: github.PullRequestActionLabeled,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment: false,
		},
	}

	SHA := "0bd3ed50c88cd53a09316bf7a298f900e9371652"

	for _, testcase := range testcases {
		tc := testcase
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakegithub.FakeClient{
				IssueComments: make(map[int][]github.IssueComment),
				PullRequests: map[int]*github.PullRequest{
					5: {
						Body: tc.body,
						Base: github.PullRequestBranch{
							Ref: "master",
						},
						Head: github.PullRequestBranch{
							SHA: SHA,
						},
						User:   github.User{Login: "author"},
						Number: 5,
						State:  "open",
					},
				},
			}

			e := &github.PullRequestEvent{
				Action:      tc.action,
				Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				PullRequest: *fc.PullRequests[5],
			}

			cfg := &externalplugins.Configuration{}
			cfg.TiCommunityAutoresponder = []externalplugins.TiCommunityAutoresponder{
				{
					Repos:        []string{"org/repo"},
					AutoResponds: tc.responds,
				},
			}

			if err := HandlePullRequestEvent(fc, e, cfg, logrus.WithField("plugin", PluginName)); err != nil {
				t.Errorf("didn't expect error from %s: %v", PluginName, err)
			}

			if tc.shouldComment && len(fc.IssueComments[5]) != tc.expectCommentNumber {
				t.Errorf("comments number mismatch: got %v, want %v", len(fc.IssueComments[5]), tc.expectCommentNumber)
			}
		})
	}
}

func TestAutoRespondIssue(t *testing.T) {
	var testcases = []struct {
		name                string
		body                string
		action              github.IssueEventAction
		responds            []externalplugins.AutoRespond
		shouldComment       bool
		expectCommentNumber int
	}{
		{
			name:   "non-matching comment",
			body:   "uh oh",
			action: github.IssueActionOpened,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment: false,
		},
		{
			name:   "matching comment",
			body:   "/merge",
			action: github.IssueActionOpened,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 1,
		},
		{
			name:   "matching comment with trailing space",
			body:   "/merge \r",
			action: github.IssueActionOpened,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   "(?mi)^/merge\\s*$",
					Message: "Got a merge command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 1,
		},
		{
			name: "matching comment with multiple auto responds",
			body: `/merge
		
                          /test`,
			action: github.IssueActionOpened,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   "(?mi)^/merge\\s*$",
					Message: "Got a merge command.",
				},
				{
					Regex:   `/test`,
					Message: "Got a test command.",
				},
				{
					Regex:   `/foo`,
					Message: "Got a foo command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 2,
		},
		{
			name:   "edited action",
			body:   "/merge",
			action: github.IssueActionEdited,
			responds: []externalplugins.AutoRespond{
				{
					Regex:   `(?mi)^/merge\s*$`,
					Message: "Got a merge command.",
				},
			},
			shouldComment:       true,
			expectCommentNumber: 1,
		},
	}

	for _, testcase := range testcases {
		tc := testcase
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakegithub.FakeClient{
				IssueComments: make(map[int][]github.IssueComment),
			}

			e := &github.IssueEvent{
				Issue: github.Issue{
					User:    github.User{Login: "author"},
					Number:  5,
					State:   "open",
					HTMLURL: "<URL>",
					Body:    tc.body,
				},
				Action: tc.action,
				Repo:   github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			}

			cfg := &externalplugins.Configuration{}
			cfg.TiCommunityAutoresponder = []externalplugins.TiCommunityAutoresponder{
				{
					Repos:        []string{"org/repo"},
					AutoResponds: tc.responds,
				},
			}

			if err := HandleIssueEvent(fc, e, cfg, logrus.WithField("plugin", PluginName)); err != nil {
				t.Errorf("didn't expect error from %s: %v", PluginName, err)
			}

			if tc.shouldComment && len(fc.IssueComments[5]) != tc.expectCommentNumber {
				t.Errorf("comments number mismatch: got %v, want %v", len(fc.IssueComments[5]), tc.expectCommentNumber)
			}
		})
	}
}

func TestHelpProvider(t *testing.T) {
	enabledRepos := []config.OrgRepo{
		{Org: "org1", Repo: "repo"},
		{Org: "org2", Repo: "repo"},
	}
	testcases := []struct {
		name               string
		config             *externalplugins.Configuration
		enabledRepos       []config.OrgRepo
		err                bool
		configInfoIncludes []string
		configInfoExcludes []string
	}{
		{
			name:               "Empty config",
			config:             &externalplugins.Configuration{},
			enabledRepos:       enabledRepos,
			configInfoExcludes: []string{":"},
		},
		{
			name: "All configs enabled",
			config: &externalplugins.Configuration{
				TiCommunityAutoresponder: []externalplugins.TiCommunityAutoresponder{
					{
						Repos: []string{"org2/repo"},
						AutoResponds: []externalplugins.AutoRespond{
							{
								Regex:   "/merge",
								Message: "Got a merge comment.",
							},
						},
					},
				},
			},
			enabledRepos:       enabledRepos,
			configInfoIncludes: []string{":"},
		},
	}
	for _, testcase := range testcases {
		tc := testcase
		t.Run(tc.name, func(t *testing.T) {
			epa := &externalplugins.ConfigAgent{}
			epa.Set(tc.config)

			helpProvider := HelpProvider(epa)
			pluginHelp, err := helpProvider(tc.enabledRepos)
			if err != nil && !tc.err {
				t.Fatalf("helpProvider error: %v", err)
			}
			for _, msg := range tc.configInfoExcludes {
				if strings.Contains(pluginHelp.Config["org2/repo"], msg) {
					t.Fatalf("helpProvider.Config error mismatch: got %v, but didn't want it", msg)
				}
			}
			for _, msg := range tc.configInfoIncludes {
				if !strings.Contains(pluginHelp.Config["org2/repo"], msg) {
					t.Fatalf("helpProvider.Config error mismatch: didn't get %v, but wanted it", msg)
				}
			}
		})
	}
}
