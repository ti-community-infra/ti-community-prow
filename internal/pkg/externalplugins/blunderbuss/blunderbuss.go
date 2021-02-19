package blunderbuss

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/ti-community-infra/tichi/internal/pkg/externalplugins"
	"github.com/ti-community-infra/tichi/internal/pkg/ownersclient"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pkg/layeredsets"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/assign"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName = "ti-community-blunderbuss"
)

var (
	match = regexp.MustCompile(`(?mi)^/auto-cc\s*$`)
)

var sleep = time.Sleep

type githubClient interface {
	RequestReview(org, repo string, number int, logins []string) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

// HelpProvider constructs the PluginHelp for this plugin that takes into account enabled repositories.
// HelpProvider defines the type for function that construct the PluginHelp for plugins.
func HelpProvider(epa *externalplugins.ConfigAgent) func(
	enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	return func(enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
		configInfo := map[string]string{}
		cfg := epa.Config()

		for _, repo := range enabledRepos {
			opts := cfg.BlunderbussFor(repo.Org, repo.Repo)
			var isConfigured bool
			var configInfoStrings []string
			configInfoStrings = append(configInfoStrings, "The plugin has these configurations:<ul>")
			if opts.MaxReviewerCount > 0 {
				configInfoStrings = append(configInfoStrings, "<li>"+configString(opts.MaxReviewerCount)+"</li>")
				isConfigured = true
			}
			configInfoStrings = append(configInfoStrings, "</ul>")
			if isConfigured {
				configInfo[repo.String()] = strings.Join(configInfoStrings, "\n")
			}
		}
		yamlSnippet, err := plugins.CommentMap.GenYaml(&externalplugins.Configuration{
			TiCommunityBlunderbuss: []externalplugins.TiCommunityBlunderbuss{
				{
					Repos:              []string{"ti-community-infra/test-dev"},
					MaxReviewerCount:   2,
					ExcludeReviewers:   []string{},
					PullOwnersEndpoint: "https://bots.tidb.io/ti-community-bot",
				},
			},
		})
		if err != nil {
			logrus.WithError(err).Warnf("cannot generate comments for %s plugin", PluginName)
		}
		pluginHelp := &pluginhelp.PluginHelp{
			Description: "The blunderbuss plugin automatically requests reviews from reviewers when a new PR is created or " +
				"when a sig label is labeled.",
			Config:  configInfo,
			Snippet: yamlSnippet,
		}
		pluginHelp.AddCommand(pluginhelp.Command{
			Usage:       "/auto-cc",
			Featured:    false,
			Description: "Manually request reviews from reviewers for a PR.",
			Examples:    []string{"/auto-cc"},
			WhoCanUse:   "Everyone",
		})
		return pluginHelp, nil
	}
}

func configString(maxReviewerCount int) string {
	var pluralSuffix string
	if maxReviewerCount > 1 {
		pluralSuffix = "s"
	}
	return fmt.Sprintf("Blunderbuss is currently configured to request reviews from %d reviewer%s.",
		maxReviewerCount, pluralSuffix)
}

// HandleIssueCommentEvent handles a GitHub pull request event and requests review.
func HandlePullRequestEvent(gc githubClient, pe *github.PullRequestEvent,
	cfg *externalplugins.Configuration, ol ownersclient.OwnersLoader, log *logrus.Entry) error {
	pr := &pe.PullRequest
	// If a PR already has reviewers, we do not automatically assign them.
	if len(pr.RequestedReviewers) > 0 {
		return nil
	}

	repo := &pe.Repo
	opts := cfg.BlunderbussFor(repo.Owner.Login, repo.Name)
	// If there is already /cc, the author has specified reviewers.
	prBodyWithoutCcCommand := !assign.CCRegexp.MatchString(pr.Body)

	isPrLabeledEvent := pe.Action == github.PullRequestActionLabeled
	openPrWithSigLabel := pe.PullRequest.State == "open" && strings.Contains(pe.Label.Name, externalplugins.SigPrefix)

	// Only handle the event of add SIG label to the open PR.
	if isPrLabeledEvent && openPrWithSigLabel && prBodyWithoutCcCommand {
		return handle(
			gc,
			opts,
			repo,
			pr,
			log,
			ol,
		)
	}

	isPrOpenedEvent := pe.Action == github.PullRequestActionOpened
	repoNonRequireSigLabel := !opts.RequireSigLabel

	// Only handle the event of opening non-CC PR, when the require_sig_label option is not turned on.
	if isPrOpenedEvent && repoNonRequireSigLabel && prBodyWithoutCcCommand {
		// Wait a few seconds to allow other automation plugin to apply labels (Mainly SIG label).
		gracePeriod := time.Duration(opts.GracePeriodDuration) * time.Second
		sleep(gracePeriod)

		// Reacquire new added labels of PR.
		labels, err := gc.GetIssueLabels(repo.Owner.Login, repo.Name, pr.Number)
		if err != nil {
			return fmt.Errorf("error loading PullRequest labels: %v", err)
		}

		// The task requesting review has been processed in the labeled event, and the open event
		// does not need to be processed repeatedly.
		if containSigLabel(labels) {
			return nil
		}

		return handle(
			gc,
			opts,
			repo,
			pr,
			log,
			ol,
		)
	}

	return nil
}

// HandleIssueCommentEvent handles a GitHub issue comment event and requests review.
func HandleIssueCommentEvent(gc githubClient, ce *github.IssueCommentEvent, cfg *externalplugins.Configuration,
	ol ownersclient.OwnersLoader, log *logrus.Entry) error {
	// Only consider open PRs and new comments.
	if ce.Action != github.IssueCommentActionCreated || !ce.Issue.IsPullRequest() || ce.Issue.State == "closed" {
		return nil
	}

	if !match.MatchString(ce.Comment.Body) {
		return nil
	}

	repo := &ce.Repo

	pr, err := gc.GetPullRequest(repo.Owner.Login, repo.Name, ce.Issue.Number)
	if err != nil {
		return fmt.Errorf("error loading PullRequest: %v", err)
	}

	opts := cfg.BlunderbussFor(repo.Owner.Login, repo.Name)

	// Check if PR has sig label.
	if opts.RequireSigLabel && !containSigLabel(pr.Labels) {
		log.Infof("the repo %v require the PR contains the sig label, but PR %v did not", repo.FullName, pr.Number)
		return nil
	}

	return handle(
		gc,
		opts,
		repo,
		pr,
		log,
		ol,
	)
}

func handle(ghc githubClient, opts *externalplugins.TiCommunityBlunderbuss, repo *github.Repo, pr *github.PullRequest,
	log *logrus.Entry, ol ownersclient.OwnersLoader) error {
	owners, err := ol.LoadOwners(opts.PullOwnersEndpoint, repo.Owner.Login, repo.Name, pr.Number)
	if err != nil {
		return fmt.Errorf("error loading RepoOwners: %v", err)
	}

	reviewers := getReviewers(pr.User.Login, owners.Reviewers, opts.ExcludeReviewers, log)
	maxReviewerCount := opts.MaxReviewerCount

	// If the maximum count of reviewers greater than 0, it needs to be split.
	if maxReviewerCount > 0 && len(reviewers) > maxReviewerCount {
		log.Infof("Limiting request of %d reviewers to %d maxReviewers.", len(reviewers), maxReviewerCount)
		reviewers = reviewers[:maxReviewerCount]
	}

	if len(reviewers) > 0 {
		log.Infof("Requesting reviews from users %s.", reviewers)
		return ghc.RequestReview(repo.Owner.Login, repo.Name, pr.Number, reviewers)
	}
	return nil
}

func getReviewers(author string, reviewers []string, excludeReviewers []string, log *logrus.Entry) []string {
	authorSet := sets.NewString(github.NormLogin(author))
	excludeReviewersSet := sets.NewString(excludeReviewers...)
	reviewersSet := sets.NewString()
	reviewersSet.Insert(reviewers...)

	var result []string
	// Exclude the author.
	availableReviewers := layeredsets.NewString(
		reviewersSet.Difference(authorSet).Difference(excludeReviewersSet).List()...)

	for availableReviewers.Len() > 0 {
		reviewer := availableReviewers.PopRandom()
		result = append(result, reviewer)
		log.Infof("Added %s as reviewers. %d reviewers found.", reviewer, len(result))
	}

	return result
}

func containSigLabel(labels []github.Label) bool {
	for _, label := range labels {
		if strings.HasPrefix(label.Name, externalplugins.SigPrefix) {
			return true
		}
	}

	return false
}
