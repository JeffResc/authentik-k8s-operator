package authentik

import (
	"context"
	"fmt"
	"net/http"

	api "goauthentik.io/api/v3"
)

const (
	// eventTransportName is the name used for the operator's webhook transport in Authentik.
	eventTransportName = "authentik-k8s-operator-webhook"
	// eventRuleName is the name used for the operator's notification rule in Authentik.
	eventRuleName = "authentik-k8s-operator-model-events"
)

// eventMatcherPolicies defines the event matcher policies the operator creates.
// One per action type so that model_created, model_updated, and model_deleted
// events all trigger the webhook.
var eventMatcherActions = []api.EventActions{
	api.EVENTACTIONS_MODEL_CREATED,
	api.EVENTACTIONS_MODEL_UPDATED,
	api.EVENTACTIONS_MODEL_DELETED,
}

// EnsureEventWebhookConfig ensures the Authentik notification transport, event
// matcher policies, notification rule, and policy bindings are created and
// up-to-date so that Authentik forwards model change events to the operator's
// webhook receiver endpoint. If secret is non-empty, a property mapping is
// created to send an Authorization header with the webhook requests.
func (c *APIClient) EnsureEventWebhookConfig(ctx context.Context, webhookURL, secret string) error {
	var headersMappingPk string
	if secret != "" {
		pk, err := c.ensureWebhookHeadersMapping(ctx, secret)
		if err != nil {
			return fmt.Errorf("ensure webhook headers mapping: %w", err)
		}
		headersMappingPk = pk
	}

	transportPk, err := c.ensureTransport(ctx, webhookURL, headersMappingPk)
	if err != nil {
		return fmt.Errorf("ensure transport: %w", err)
	}

	rulePk, err := c.ensureRule(ctx, transportPk)
	if err != nil {
		return fmt.Errorf("ensure rule: %w", err)
	}

	if err := c.ensureEventMatcherPolicies(ctx, rulePk); err != nil {
		return fmt.Errorf("ensure event matcher policies: %w", err)
	}

	return nil
}

// CleanupEventWebhookConfig removes the notification transport, rule, policies,
// and bindings managed by the operator.
func (c *APIClient) CleanupEventWebhookConfig(ctx context.Context) error {
	// Delete policies first (bindings are cascade-deleted with the rule)
	for _, action := range eventMatcherActions {
		policyName := eventMatcherPolicyName(action)
		policies, _, err := c.api.PoliciesAPI.PoliciesEventMatcherList(ctx).Name(policyName).Execute()
		if err != nil {
			return fmt.Errorf("list policy %s: %w", policyName, err)
		}
		for _, p := range policies.Results {
			resp, err := c.api.PoliciesAPI.PoliciesEventMatcherDestroy(ctx, p.Pk).Execute()
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusMethodNotAllowed {
					continue
				}
				return fmt.Errorf("delete policy %s: %w", policyName, err)
			}
		}
	}

	// Delete rule (tolerate 405 — some Authentik versions don't allow rule deletion)
	rules, _, err := c.api.EventsAPI.EventsRulesList(ctx).Name(eventRuleName).Execute()
	if err != nil {
		return fmt.Errorf("list rules: %w", err)
	}
	for _, r := range rules.Results {
		resp, err := c.api.EventsAPI.EventsRulesDestroy(ctx, r.Pk).Execute()
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusMethodNotAllowed {
				// Rule deletion not supported by this Authentik version — skip
				continue
			}
			return fmt.Errorf("delete rule: %w", err)
		}
	}

	// Delete transport (tolerate 405 for the same reason)
	transports, _, err := c.api.EventsAPI.EventsTransportsList(ctx).Name(eventTransportName).Execute()
	if err != nil {
		return fmt.Errorf("list transports: %w", err)
	}
	for _, t := range transports.Results {
		resp, err := c.api.EventsAPI.EventsTransportsDestroy(ctx, t.Pk).Execute()
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusMethodNotAllowed {
				continue
			}
			return fmt.Errorf("delete transport: %w", err)
		}
	}

	return nil
}

func (c *APIClient) ensureTransport(ctx context.Context, webhookURL, headersMappingPk string) (string, error) {
	mode := api.TRANSPORTMODEENUM_WEBHOOK
	sendOnce := true

	buildReq := func() *api.NotificationTransportRequest {
		req := api.NewNotificationTransportRequest(eventTransportName)
		req.SetMode(mode)
		req.SetWebhookUrl(webhookURL)
		req.SetSendOnce(sendOnce)
		if headersMappingPk != "" {
			req.SetWebhookMappingHeaders(headersMappingPk)
		}
		return req
	}

	transports, _, err := c.api.EventsAPI.EventsTransportsList(ctx).Name(eventTransportName).Execute()
	if err != nil {
		return "", fmt.Errorf("list transports: %w", err)
	}

	if len(transports.Results) > 0 {
		existing := transports.Results[0]
		req := buildReq()
		updated, resp, err := c.api.EventsAPI.EventsTransportsUpdate(ctx, existing.Pk).NotificationTransportRequest(*req).Execute()
		if err != nil {
			return "", extractAPIError(err, "update transport")
		}
		if resp != nil && resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("update transport: status %d", resp.StatusCode)
		}
		return updated.Pk, nil
	}

	// Create new transport
	req := buildReq()
	transport, resp, err := c.api.EventsAPI.EventsTransportsCreate(ctx).NotificationTransportRequest(*req).Execute()
	if err != nil {
		return "", extractAPIError(err, "create transport")
	}
	if resp != nil && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create transport: status %d", resp.StatusCode)
	}
	return transport.Pk, nil
}

const webhookHeadersMappingName = "authentik-k8s-operator-webhook-headers"

func (c *APIClient) ensureWebhookHeadersMapping(ctx context.Context, secret string) (string, error) {
	expression := fmt.Sprintf(`return {"Authorization": "Bearer %s"}`, secret)

	mappings, _, err := c.api.PropertymappingsAPI.PropertymappingsNotificationList(ctx).Name(webhookHeadersMappingName).Execute()
	if err != nil {
		return "", fmt.Errorf("list webhook header mappings: %w", err)
	}

	if len(mappings.Results) > 0 {
		existing := mappings.Results[0]
		req := api.NewNotificationWebhookMappingRequest(webhookHeadersMappingName, expression)
		updated, resp, err := c.api.PropertymappingsAPI.PropertymappingsNotificationUpdate(ctx, existing.Pk).NotificationWebhookMappingRequest(*req).Execute()
		if err != nil {
			return "", extractAPIError(err, "update webhook headers mapping")
		}
		if resp != nil && resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("update webhook headers mapping: status %d", resp.StatusCode)
		}
		return updated.Pk, nil
	}

	req := api.NewNotificationWebhookMappingRequest(webhookHeadersMappingName, expression)
	mapping, resp, err := c.api.PropertymappingsAPI.PropertymappingsNotificationCreate(ctx).NotificationWebhookMappingRequest(*req).Execute()
	if err != nil {
		return "", extractAPIError(err, "create webhook headers mapping")
	}
	if resp != nil && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create webhook headers mapping: status %d", resp.StatusCode)
	}
	return mapping.Pk, nil
}

func (c *APIClient) ensureRule(ctx context.Context, transportPk string) (string, error) {
	severity := api.SEVERITYENUM_NOTICE

	rules, _, err := c.api.EventsAPI.EventsRulesList(ctx).Name(eventRuleName).Execute()
	if err != nil {
		return "", fmt.Errorf("list rules: %w", err)
	}

	if len(rules.Results) > 0 {
		existing := rules.Results[0]
		// Update to ensure transport reference is correct
		req := api.NewNotificationRuleRequest(eventRuleName)
		req.SetTransports([]string{transportPk})
		req.SetSeverity(severity)
		updated, resp, err := c.api.EventsAPI.EventsRulesUpdate(ctx, existing.Pk).NotificationRuleRequest(*req).Execute()
		if err != nil {
			return "", extractAPIError(err, "update rule")
		}
		if resp != nil && resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("update rule: status %d", resp.StatusCode)
		}
		return updated.Pk, nil
	}

	// Create new rule
	req := api.NewNotificationRuleRequest(eventRuleName)
	req.SetTransports([]string{transportPk})
	req.SetSeverity(severity)
	rule, resp, err := c.api.EventsAPI.EventsRulesCreate(ctx).NotificationRuleRequest(*req).Execute()
	if err != nil {
		return "", extractAPIError(err, "create rule")
	}
	if resp != nil && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create rule: status %d", resp.StatusCode)
	}
	return rule.Pk, nil
}

func eventMatcherPolicyName(action api.EventActions) string {
	return fmt.Sprintf("authentik-k8s-operator-%s", string(action))
}

func (c *APIClient) ensureEventMatcherPolicies(ctx context.Context, rulePk string) error {
	for _, action := range eventMatcherActions {
		policyPk, err := c.ensureSingleEventMatcher(ctx, action)
		if err != nil {
			return err
		}
		if err := c.ensurePolicyBinding(ctx, policyPk, rulePk); err != nil {
			return err
		}
	}
	return nil
}

func (c *APIClient) ensureSingleEventMatcher(ctx context.Context, action api.EventActions) (string, error) {
	name := eventMatcherPolicyName(action)

	policies, _, err := c.api.PoliciesAPI.PoliciesEventMatcherList(ctx).Name(name).Execute()
	if err != nil {
		return "", fmt.Errorf("list event matcher policies: %w", err)
	}

	if len(policies.Results) > 0 {
		return policies.Results[0].Pk, nil
	}

	// Create new event matcher policy
	req := api.NewEventMatcherPolicyRequest(name)
	req.SetAction(action)
	policy, resp, err := c.api.PoliciesAPI.PoliciesEventMatcherCreate(ctx).EventMatcherPolicyRequest(*req).Execute()
	if err != nil {
		return "", extractAPIError(err, fmt.Sprintf("create event matcher policy %s", name))
	}
	if resp != nil && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create event matcher policy %s: status %d", name, resp.StatusCode)
	}
	return policy.Pk, nil
}

func (c *APIClient) ensurePolicyBinding(ctx context.Context, policyPk, rulePk string) error {
	// Check if binding already exists
	bindings, _, err := c.api.PoliciesAPI.PoliciesBindingsList(ctx).Target(rulePk).Execute()
	if err != nil {
		return fmt.Errorf("list policy bindings: %w", err)
	}

	for _, b := range bindings.Results {
		if b.Policy.Get() != nil && *b.Policy.Get() == policyPk {
			return nil // Already bound
		}
	}

	// Create binding
	req := api.NewPolicyBindingRequest(rulePk, int32(len(bindings.Results)))
	req.SetPolicy(policyPk)
	req.SetEnabled(true)
	_, resp, err := c.api.PoliciesAPI.PoliciesBindingsCreate(ctx).PolicyBindingRequest(*req).Execute()
	if err != nil {
		return extractAPIError(err, "create policy binding")
	}
	if resp != nil && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("create policy binding: status %d", resp.StatusCode)
	}
	return nil
}
