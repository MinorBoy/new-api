package service

import (
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type blockingRefundFunding struct {
	started chan struct{}
	release chan struct{}
}

func (funding *blockingRefundFunding) Source() string {
	return BillingSourceWallet
}

func (funding *blockingRefundFunding) PreConsume(int) error {
	return nil
}

func (funding *blockingRefundFunding) Settle(int) error {
	return nil
}

func (funding *blockingRefundFunding) Refund() error {
	close(funding.started)
	<-funding.release
	return nil
}

func TestBillingSessionRefundWaitsForFundingRefund(t *testing.T) {
	funding := &blockingRefundFunding{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			UserId:       1,
			IsPlayground: true,
		},
		funding:       funding,
		tokenConsumed: 1,
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	returned := make(chan struct{})
	go func() {
		session.Refund(ctx)
		close(returned)
	}()

	select {
	case <-funding.started:
	case <-time.After(time.Second):
		t.Fatal("funding refund did not start")
	}
	select {
	case <-returned:
		t.Fatal("Refund returned before the funding refund completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(funding.release)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Refund did not return after the funding refund completed")
	}
	require.False(t, session.NeedsRefund())
}
