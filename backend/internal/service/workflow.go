package service

import (
	"context"
	"errors"
	"github.com/uptrace/bun"
	"sre_tbank/internal/store"
)

type TicketActionInput struct {
	Action      string  `json:"action"`
	Message     *string `json:"message"`
	AssigneeID  *int64  `json:"assignee_id"`
	AssigneeSet bool    `json:"-"`
}

// TicketAction holds the parent row lock through both the transition and its
// conversation message. Claims and requester confirmations cannot race each other.
func (s *Service) TicketAction(ctx context.Context, u store.User, id int64, in TicketActionInput) (result store.Ticket, err error) {
	if !valid(in.Action, "claim", "reassign", "resolve", "confirm", "request_changes", "reopen") {
		return result, Bad("Unknown ticket action")
	}

	needsMessage := valid(in.Action, "resolve", "request_changes", "reopen")
	if needsMessage != (in.Message != nil) {
		return result, Bad("This action requires a resolution or reason; other actions do not accept a message")
	}

	if (in.Action == "reassign") != in.AssigneeSet {
		return result, Bad("Only reassignment accepts assignee_id, including null to return to the queue")
	}

	var message string
	if needsMessage {
		var err error
		message, err = text(*in.Message, 5000, true)
		if err != nil {
			return result, err
		}
	}

	defer func() { businessAttempt(err, func() { s.Metrics.TicketActionAttempt(in.Action) }) }()
	err = s.Store.MutateTicket(ctx, u, id, func(ctx context.Context, tx bun.Tx, v *store.Ticket) error {
		if valid(in.Action, "confirm", "request_changes") {
			if v.RequesterID != u.ID {
				return &Error{Status: 403, Code: "forbidden", Message: "Only the requester can confirm the outcome"}
			}
		} else if err := Operator(u); err != nil {
			return err
		}

		switch in.Action {
		case "claim":
			if !valid(v.Status, "open", "in_progress") || v.AssigneeID != nil {
				return conflict("This ticket has already been assigned or is no longer available; refresh the queue")
			}

			v.AssigneeID = &u.ID
			v.Status = "in_progress"
		case "reassign":
			if !valid(v.Status, "open", "in_progress") {
				return conflict("Reopen this ticket before changing its assignment")
			}

			if in.AssigneeID != nil {
				var assignee store.User
				err := tx.NewSelect().Model(&assignee).Where("id=? AND role='operator'", *in.AssigneeID).Scan(ctx)
				if errors.Is(store.Translate(err), store.ErrNotFound) {
					return Bad("Assignee must be an existing operator")
				}

				if err != nil {
					return err
				}

				v.Status = "in_progress"
			} else {
				v.Status = "open"
			}

			v.AssigneeID = in.AssigneeID
		case "resolve":
			if v.Status != "in_progress" || v.AssigneeID == nil {
				return conflict("Take or assign this ticket before resolving it")
			}

			v.Status = "resolved"
		case "confirm":
			if v.Status != "resolved" {
				return conflict("Only a resolved ticket can be confirmed")
			}

			v.Status = "closed"
			message = "The requester confirmed that the issue is fixed."
		case "request_changes":
			if v.Status != "resolved" {
				return conflict("Feedback on the outcome is available only for resolved tickets")
			}

			v.Status = "in_progress"
		case "reopen":
			if !valid(v.Status, "resolved", "closed") {
				return conflict("Only resolved or closed tickets can be reopened")
			}

			if v.AssigneeID == nil {
				v.Status = "open"
			} else {
				v.Status = "in_progress"
			}
		}

		if err := s.Faults.Apply(ctx); err != nil {
			return err
		}
		if message != "" {
			comment := store.Comment{TicketID: v.ID, AuthorID: u.ID, Body: message}
			if err := store.InsertComment(ctx, tx, &comment); err != nil {
				return err
			}
		}

		if err := store.SaveTicket(ctx, tx, v); err != nil {
			return err
		}

		q, current := store.TicketQuery(tx, u, id, false)
		if err := q.Scan(ctx); err != nil {
			return err
		}

		result = *current
		return nil
	})

	if err == nil {
		s.Metrics.TicketAction(in.Action)
		if in.Action == "resolve" {
			s.Metrics.TicketResolved(result.Priority, result.UpdatedAt.Sub(result.CreatedAt))
		}
	}
	return result, Normalize(err)
}
