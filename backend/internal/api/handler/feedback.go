package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmyagentic/awesome-agent-app-features/feedback"
	feedbackhttp "github.com/timmyagentic/awesome-agent-app-features/feedback/httpclient"
)

var (
	errFeedbackApprovalRequired = errors.New("explicit feedback approval is required")
	errFeedbackRelayUnavailable = errors.New("feedback relay is unavailable; use the public fallback")
)

// FeedbackHostConfig contains only non-secret host policy. The Relay owns its
// downstream repository and credentials.
type FeedbackHostConfig struct {
	Endpoint          string
	PublicFallbackURL string
	ProductVersion    string
}

type feedbackSubmitter interface {
	Submit(context.Context, feedback.Approved) (feedbackhttp.Receipt, error)
}

// FeedbackHandler owns Emomo's host rendering boundary and explicit approval
// action while delegating redaction and transport safety to the Foundation.
type FeedbackHandler struct {
	config    FeedbackHostConfig
	submitter feedbackSubmitter
}

// NewFeedbackHandler constructs an optional Relay-backed host adapter.
func NewFeedbackHandler(config FeedbackHostConfig, submitters ...feedbackSubmitter) *FeedbackHandler {
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	config.PublicFallbackURL = normalizePublicFallbackURL(config.PublicFallbackURL)
	config.ProductVersion = strings.TrimSpace(config.ProductVersion)
	if config.ProductVersion == "" {
		config.ProductVersion = "development"
	}
	var submitter feedbackSubmitter
	if len(submitters) > 0 {
		submitter = submitters[0]
	} else if config.Endpoint != "" {
		submitter = feedbackhttp.Client{
			Endpoint:  config.Endpoint,
			UserAgent: "emomo-feedback/" + config.ProductVersion,
		}
	}
	return &FeedbackHandler{config: config, submitter: submitter}
}

func normalizePublicFallbackURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ""
	}
	return parsed.String()
}

// Preview returns the complete redacted report without approving or sending it.
func (h *FeedbackHandler) Preview(c *gin.Context) {
	var request pb.FeedbackPreviewRequest
	if err := readProtoJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	_, report, err := h.buildDraft(request.Description)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	writeProtoJSON(c, http.StatusOK, &pb.FeedbackPreviewResponse{
		Preview:           feedbackReportToProto(report),
		SubmissionEnabled: h.submitter != nil,
		PublicFallbackUrl: h.config.PublicFallbackURL,
	})
}

// Submit rebuilds the exact deterministic preview and approves it only when
// user_approved came from the host's explicit confirmation action.
func (h *FeedbackHandler) Submit(c *gin.Context) {
	var request pb.FeedbackSubmitRequest
	if err := readProtoJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	if !request.UserApproved {
		writeError(c, http.StatusBadRequest, errFeedbackApprovalRequired)
		return
	}
	if h.submitter == nil {
		writeError(c, http.StatusServiceUnavailable, errFeedbackRelayUnavailable)
		return
	}
	draft, _, err := h.buildDraft(request.Description)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	approved, err := draft.Approve(true)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	receipt, err := h.submitter.Submit(c.Request.Context(), approved)
	if err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}
	writeProtoJSON(c, http.StatusOK, &pb.FeedbackSubmitResponse{
		ReferenceUrl: receipt.ReferenceURL,
		Deduplicated: receipt.Deduplicated,
	})
}

func (h *FeedbackHandler) buildDraft(description string) (feedback.Draft, feedback.Report, error) {
	draft, err := (feedback.Builder{}).Build(feedback.Input{
		Description: description,
		Environment: feedback.Environment{
			Product: "emomo",
			Version: h.config.ProductVersion,
			Agent:   "semantic-search",
		},
	})
	if err != nil {
		return feedback.Draft{}, feedback.Report{}, err
	}
	return draft, draft.Report(), nil
}

func feedbackReportToProto(report feedback.Report) *pb.FeedbackPreview {
	preview := &pb.FeedbackPreview{
		Environment: &pb.FeedbackEnvironment{
			Product: report.Environment.Product,
			Version: report.Environment.Version,
			Os:      report.Environment.OS,
			Arch:    report.Environment.Arch,
			Agent:   report.Environment.Agent,
		},
		Description:    report.Description,
		CapabilityGaps: append([]string(nil), report.CapabilityGaps...),
	}
	if report.RecentError != nil {
		preview.RecentError = &pb.FeedbackRecentError{
			Text:       report.RecentError.Text,
			OccurredAt: report.RecentError.At.UTC().Format(time.RFC3339),
		}
	}
	return preview
}
