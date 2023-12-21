package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	decodepay "github.com/nbd-wtf/ln-decodepay"
	"github.com/sirupsen/logrus"
)

func (svc *Service) HandlePayInvoiceEvent(ctx context.Context, request *Nip47Request, event *nostr.Event, app App, ss []byte) (result *nostr.Event, err error) {

	Client := svc.lnClient
	if app.BackendOptions.Backend == "lnbits" {
		svc.lnClient = setClientLNBits(app, svc)
	}

	nostrEvent := NostrEvent{App: app, NostrId: event.ID, Content: event.Content, State: "received"}
	err = svc.db.Create(&nostrEvent).Error
	if err != nil {
		svc.Logger.WithFields(logrus.Fields{
			"eventId":   event.ID,
			"eventKind": event.Kind,
			"appId":     app.ID,
		}).Errorf("Failed to save nostr event: %v", err)
		return nil, err
	}

	var bolt11 string
	payParams := &Nip47PayParams{}
	err = json.Unmarshal(request.Params, payParams)
	if err != nil {
		svc.Logger.WithFields(logrus.Fields{
			"eventId":   event.ID,
			"eventKind": event.Kind,
			"appId":     app.ID,
		}).Errorf("Failed to decode nostr event: %v", err)
		return nil, err
	}

	bolt11 = payParams.Invoice
	// Convert invoice to lowercase string
	bolt11 = strings.ToLower(bolt11)
	paymentRequest, err := decodepay.Decodepay(bolt11)
	if err != nil {
		svc.Logger.WithFields(logrus.Fields{
			"eventId":   event.ID,
			"eventKind": event.Kind,
			"appId":     app.ID,
			"bolt11":    bolt11,
		}).Errorf("Failed to decode bolt11 invoice: %v", err)

		return svc.createResponse(event, Nip47Response{
			ResultType: NIP_47_PAY_INVOICE_METHOD,
			Error: &Nip47Error{
				Code:    NIP_47_ERROR_INTERNAL,
				Message: fmt.Sprintf("Failed to decode bolt11 invoice: %s", err.Error()),
			},
		}, ss)
	}

	hasPermission, code, message := svc.hasPermission(&app, event, request.Method, paymentRequest.MSatoshi)

	if !hasPermission {
		svc.Logger.WithFields(logrus.Fields{
			"eventId":   event.ID,
			"eventKind": event.Kind,
			"appId":     app.ID,
		}).Errorf("App does not have permission: %s %s", code, message)

		return svc.createResponse(event, Nip47Response{
			ResultType: NIP_47_PAY_INVOICE_METHOD,
			Error: &Nip47Error{
				Code:    code,
				Message: message,
			}}, ss)
	}

	payment := Payment{App: app, NostrEvent: nostrEvent, PaymentRequest: bolt11, Amount: uint(paymentRequest.MSatoshi / 1000)}
	insertPaymentResult := svc.db.Create(&payment)
	if insertPaymentResult.Error != nil {
		return nil, insertPaymentResult.Error
	}

	svc.Logger.WithFields(logrus.Fields{
		"eventId":   event.ID,
		"eventKind": event.Kind,
		"appId":     app.ID,
		"bolt11":    bolt11,
	}).Info("Sending payment")

	Client := svc.lnClient
	if app.BackendOptions.Backend == "lnbits" {
		var lnbitsClient *LNClient
		var host = app.BackendOptions.LNBitsHost
		if app.BackendOptions.LNBitsHost == "" {
			if svc.cfg.LNBitsHost != "" {
				host = svc.cfg.LNBitsHost
			} else {
				host = "http://" + svc.cfg.LnBitsUmbrel + ":3007"
			}
		}
		var options = LNBitsOptions{
			AdminKey: app.BackendOptions.LNBitsKey,
			Host:     host,
		}
		svc.lnClient = &LNBitsWrapper{lnbitsClient, options}
	}

	preimage, err := svc.lnClient.SendPaymentSync(ctx, event.PubKey, bolt11)
	if err != nil {
		svc.Logger.WithFields(logrus.Fields{
			"eventId":   event.ID,
			"eventKind": event.Kind,
			"appId":     app.ID,
			"bolt11":    bolt11,
		}).Infof("Failed to send payment: %v", err)
		nostrEvent.State = NOSTR_EVENT_STATE_HANDLER_ERROR
		svc.db.Save(&nostrEvent)
		return svc.createResponse(event, Nip47Response{
			ResultType: NIP_47_PAY_INVOICE_METHOD,
			Error: &Nip47Error{
				Code:    NIP_47_ERROR_INTERNAL,
				Message: fmt.Sprintf("Something went wrong while paying invoice: %s", err.Error()),
			},
		}, ss)
	}

	payment.Preimage = &preimage
	svc.lnClient = Client
	nostrEvent.State = NOSTR_EVENT_STATE_HANDLER_EXECUTED

	svc.db.Save(&nostrEvent)
	svc.db.Save(&payment)
	return svc.createResponse(event, Nip47Response{
		ResultType: NIP_47_PAY_INVOICE_METHOD,
		Result: Nip47PayResponse{
			Preimage: preimage,
		},
	}, ss)
}
