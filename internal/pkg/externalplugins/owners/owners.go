package owners

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	tiexternalplugins "github.com/tidb-community-bots/ti-community-prow/internal/pkg/externalplugins"
	"github.com/tidb-community-bots/ti-community-prow/internal/pkg/ownersclient"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

const (
	// SigEndpointFmt specifies a format for sigs URL.
	SigEndpointFmt = "/sigs/%s"
)

const (
	// sigPrefix is a default sig label prefix.
	sigPrefix = "sig/"
	// listOwnersSuccessMessage returns on success.
	listOwnersSuccessMessage = "List all owners success."
	lgtmTwo                  = 2
)

type githubClient interface {
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	ListCollaborators(org, repo string) ([]github.User, error)
	ListTeams(org string) ([]github.Team, error)
	ListTeamMembers(id int, role string) ([]github.TeamMember, error)
}

type Server struct {
	// Client for get sig info.
	Client *http.Client

	TokenGenerator func() []byte
	Gc             githubClient
	ConfigAgent    *tiexternalplugins.ConfigAgent
	Log            *logrus.Entry
}

func (s *Server) listOwnersForNonSig(org string, repo string,
	trustTeamMembers []string, requireLgtm int) (*ownersclient.OwnersResponse, error) {
	collaborators, err := s.Gc.ListCollaborators(org, repo)
	if err != nil {
		s.Log.WithField("org", org).WithField("repo", repo).WithError(err).Error("Failed get collaborators.")
		return nil, err
	}

	var collaboratorsLogin []string
	for _, collaborator := range collaborators {
		collaboratorsLogin = append(collaboratorsLogin, collaborator.Login)
	}
	approvers := sets.NewString(collaboratorsLogin...).Insert(trustTeamMembers...).List()

	if requireLgtm == 0 {
		requireLgtm = lgtmTwo
	}

	return &ownersclient.OwnersResponse{
		Data: ownersclient.Owners{
			Approvers: approvers,
			Reviewers: approvers,
			NeedsLgtm: requireLgtm,
		},
		Message: listOwnersSuccessMessage,
	}, nil
}

func (s *Server) listOwnersForSig(sigName string,
	opts *tiexternalplugins.TiCommunityOwners, trustTeamMembers []string,
	requireLgtm int) (*ownersclient.OwnersResponse, error) {
	url := opts.SigEndpoint + fmt.Sprintf(SigEndpointFmt, sigName)
	// Get sig info.
	res, err := s.Client.Get(url)
	if err != nil {
		s.Log.WithField("url", url).WithError(err).Error("Failed get sig info.")
		return nil, err
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != 200 {
		s.Log.WithField("url", url).WithError(err).Error("Failed get sig info.")
		return nil, errors.New("could not get a sig")
	}

	// Unmarshal sig members from body.
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var sigRes SigResponse
	if err := json.Unmarshal(body, &sigRes); err != nil {
		s.Log.WithField("body", body).WithError(err).Error("Failed unmarshal body.")
		return nil, err
	}

	var approvers []string
	var reviewers []string

	sig := sigRes.Data

	for _, leader := range sig.Membership.TechLeaders {
		approvers = append(approvers, leader.GithubName)
		reviewers = append(reviewers, leader.GithubName)
	}

	for _, coLeader := range sig.Membership.CoLeaders {
		approvers = append(approvers, coLeader.GithubName)
		reviewers = append(reviewers, coLeader.GithubName)
	}

	for _, committer := range sig.Membership.Committers {
		approvers = append(approvers, committer.GithubName)
		reviewers = append(reviewers, committer.GithubName)
	}

	for _, reviewer := range sig.Membership.Reviewers {
		reviewers = append(reviewers, reviewer.GithubName)
	}

	if requireLgtm == 0 {
		requireLgtm = sig.NeedsLgtm
	}

	return &ownersclient.OwnersResponse{
		Data: ownersclient.Owners{
			Approvers: sets.NewString(approvers...).Insert(trustTeamMembers...).List(),
			Reviewers: sets.NewString(reviewers...).Insert(trustTeamMembers...).List(),
			NeedsLgtm: requireLgtm,
		},
		Message: listOwnersSuccessMessage,
	}, nil
}

// ListOwners returns owners of tidb community PR.
func (s *Server) ListOwners(org string, repo string, number int,
	config *tiexternalplugins.Configuration) (*ownersclient.OwnersResponse, error) {
	// Get pull request.
	pull, err := s.Gc.GetPullRequest(org, repo, number)
	if err != nil {
		s.Log.WithField("pullNumber", number).WithError(err).Error("Failed get pull request.")
		return nil, err
	}

	opts := config.OwnersFor(org, repo)
	// Find sig label.
	sigName := getSigNameByLabel(pull.Labels)

	// Use default sig name if cannot find.
	if sigName == "" {
		sigName = opts.DefaultSigName
	}

	requireLgtm, err := getRequireLgtmByLabel(pull.Labels, opts.RequireLgtmLabelPrefix)
	if err != nil {
		s.Log.WithField("pullNumber", number).WithError(err).Error("Failed parse require lgtm.")
		return nil, err
	}

	trustTeamMembers := getTrustTeamMembers(s.Log, s.Gc, org, opts.OwnersTrustTeam)

	// When we cannot find a sig label for PR and there is no default sig name, we will use a collaborators.
	if sigName == "" {
		return s.listOwnersForNonSig(org, repo, trustTeamMembers, requireLgtm)
	}

	return s.listOwnersForSig(sigName, opts, trustTeamMembers, requireLgtm)
}

// getSigNameByLabel returns the name of sig when the label prefix matches.
func getSigNameByLabel(labels []github.Label) string {
	var sigName string
	for _, label := range labels {
		if strings.HasPrefix(label.Name, sigPrefix) {
			sigName = strings.TrimPrefix(label.Name, sigPrefix)
			return sigName
		}
	}

	return ""
}

// getRequireLgtmByLabel returns the number of require lgtm when the label prefix matches.
func getRequireLgtmByLabel(labels []github.Label, labelPrefix string) (int, error) {
	noRequireLgtm := 0

	if len(labelPrefix) == 0 {
		return noRequireLgtm, nil
	}

	for _, label := range labels {
		if strings.HasPrefix(label.Name, labelPrefix) {
			requireLgtm, err := strconv.Atoi(strings.TrimPrefix(label.Name, labelPrefix))
			if err != nil {
				return noRequireLgtm, err
			}
			return requireLgtm, nil
		}
	}

	return noRequireLgtm, nil
}

// getTrustTeamMembers returns the members of trust team.
func getTrustTeamMembers(log *logrus.Entry, gc githubClient, org, trustTeam string) []string {
	if len(trustTeam) > 0 {
		if teams, err := gc.ListTeams(org); err == nil {
			for _, teamInOrg := range teams {
				if strings.Compare(teamInOrg.Name, trustTeam) == 0 {
					if members, err := gc.ListTeamMembers(teamInOrg.ID, github.RoleAll); err == nil {
						var membersLogin []string
						for _, member := range members {
							membersLogin = append(membersLogin, member.Login)
						}
						return membersLogin
					}
					log.WithError(err).Errorf("Failed to list members in %s:%s.", org, teamInOrg.Name)
				}
			}
		} else {
			log.WithError(err).Errorf("Failed to list teams in org %s.", org)
		}
	}
	return []string{}
}
