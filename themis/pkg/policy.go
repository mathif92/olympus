// Package pkg contains the Themis business-logic layer: multi-tenant identity
// management (users, groups, roles, policies), credential issuance, policy
// evaluation and token minting.
package pkg

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Policy statement effects.
const (
	EffectAllow = "Allow"
	EffectDeny  = "Deny"
)

// PolicyStatement is a single rule inside a policy document.
type PolicyStatement struct {
	Sid      string   `json:"Sid,omitempty"`
	Effect   string   `json:"Effect"`
	Action   []string `json:"Action"`
	Resource []string `json:"Resource"`
}

// PolicyDocument is an IAM-style JSON policy document.
type PolicyDocument struct {
	Version   string            `json:"Version,omitempty"`
	Statement []PolicyStatement `json:"Statement"`
}

// ParsePolicyDocument decodes a JSON policy document. Documents are also
// stored verbatim, so a strict-enough subset is enforced here.
func ParsePolicyDocument(raw string) (*PolicyDocument, error) {
	var doc PolicyDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("invalid policy document: %w", err)
	}
	if len(doc.Statement) == 0 {
		return nil, fmt.Errorf("policy document must contain at least one Statement")
	}
	for i := range doc.Statement {
		s := &doc.Statement[i]
		if s.Effect != EffectAllow && s.Effect != EffectDeny {
			return nil, fmt.Errorf("statement %d: Effect must be Allow or Deny", i+1)
		}
		if len(s.Action) == 0 || len(s.Resource) == 0 {
			return nil, fmt.Errorf("statement %d: Action and Resource are required", i+1)
		}
	}
	return &doc, nil
}

// EvaluationDecision is the result of checking an action/resource against a
// principal's effective policies.
type EvaluationDecision struct {
	Allowed   bool     `json:"allowed"`
	Principal string   `json:"principal"`
	Action    string   `json:"action"`
	Resource  string   `json:"resource"`
	Matched   []string `json:"matched_statements,omitempty"`
}

// EvaluatePolicies evaluates action/resource against an ordered list of policy
// documents. The result is the sum of all attached policies: an explicit Deny
// always overrides Allow, and a Deny that matches no statement is ignored.
// If no statement matches, access is denied by default (implicit deny).
func EvaluatePolicies(docs []*PolicyDocument, action, resource string) EvaluationDecision {
	var (
		allowed bool
		denied  bool
		matched []string
	)
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		for _, st := range doc.Statement {
			if !stMatched(st, action, resource) {
				continue
			}
			label := st.Sid
			if label == "" {
				label = st.Effect
			}
			matched = append(matched, label)
			switch st.Effect {
			case EffectDeny:
				denied = true
			case EffectAllow:
				allowed = true
			}
		}
	}
	return EvaluationDecision{
		Allowed: allowed && !denied,
		Matched: matched,
	}
}

// stMatched reports whether a statement applies to the given action/resource.
// Wildcards are supported as a trailing "*" (prefix match), matching AWS-style
// action namespaces such as "iam:*" or "clio:Describe*".
func stMatched(st PolicyStatement, action, resource string) bool {
	return anyMatch(st.Action, action) && anyMatch(st.Resource, resource)
}

func anyMatch(patterns []string, value string) bool {
	for _, p := range patterns {
		if wildcardMatch(p, value) {
			return true
		}
	}
	return false
}

// wildcardMatch matches exact values and trailing "*" prefixes.
func wildcardMatch(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}
