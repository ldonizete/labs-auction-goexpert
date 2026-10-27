package auction

import (
	"context"
	"fullcycle-auction_go/internal/entity/auction_entity"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultTestMongoURL = "mongodb://admin:admin@localhost:27017/?authSource=admin"

func TestGetAuctionDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration string
		interval string
		want     time.Duration
	}{
		{"uses AUCTION_DURATION", "3s", "20s", 3 * time.Second},
		{"falls back to AUCTION_INTERVAL", "", "20s", 20 * time.Second},
		{"ignores invalid AUCTION_DURATION", "abc", "20s", 20 * time.Second},
		{"ignores non-positive AUCTION_DURATION", "-5s", "20s", 20 * time.Second},
		{"defaults when nothing is set", "", "", defaultAuctionDuration},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AUCTION_DURATION", tt.duration)
			t.Setenv("AUCTION_INTERVAL", tt.interval)

			if got := getAuctionDuration(); got != tt.want {
				t.Errorf("getAuctionDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCreateAuctionClosesAutomatically needs a running MongoDB (see README).
// It is skipped when none is reachable at MONGODB_URL_TEST.
func TestCreateAuctionClosesAutomatically(t *testing.T) {
	const duration = 2 * time.Second
	t.Setenv("AUCTION_DURATION", duration.String())

	database := connectTestDatabase(t)
	repository := NewAuctionRepository(database)

	auctionEntity, err := auction_entity.CreateAuction(
		"Notebook", "Electronics", "Notebook in perfect condition", auction_entity.New)
	if err != nil {
		t.Fatalf("building auction entity: %v", err.Message)
	}

	ctx := context.Background()
	if err := repository.CreateAuction(ctx, auctionEntity); err != nil {
		t.Fatalf("creating auction: %v", err.Message)
	}

	// Step 1: right after creation the auction is open.
	if got := findStatus(t, repository, auctionEntity.Id); got != auction_entity.Active {
		t.Fatalf("status right after creation = %v, want Active", got)
	}

	// Steps 2 and 3: wait for AUCTION_DURATION without touching the auction
	// and check that it was closed by the background goroutine.
	deadline := time.Now().Add(duration + 5*time.Second)
	for time.Now().Before(deadline) {
		if findStatus(t, repository, auctionEntity.Id) == auction_entity.Completed {
			if elapsed := time.Since(auctionEntity.Timestamp); elapsed < duration {
				t.Fatalf("auction closed after %v, before AUCTION_DURATION (%v)", elapsed, duration)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("auction still %v after waiting %v, want Completed",
		findStatus(t, repository, auctionEntity.Id), duration)
}

func findStatus(
	t *testing.T,
	repository *AuctionRepository,
	auctionId string) auction_entity.AuctionStatus {
	t.Helper()

	found, err := repository.FindAuctionById(context.Background(), auctionId)
	if err != nil {
		t.Fatalf("finding auction: %v", err.Message)
	}
	return found.Status
}

func connectTestDatabase(t *testing.T) *mongo.Database {
	t.Helper()

	mongoURL := os.Getenv("MONGODB_URL_TEST")
	if mongoURL == "" {
		mongoURL = defaultTestMongoURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURL))
	if err != nil {
		t.Skipf("mongodb unavailable: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("mongodb unavailable at %q: %v", mongoURL, err)
	}

	database := client.Database("auctions_test")
	t.Cleanup(func() {
		ctx := context.Background()
		_ = database.Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	return database
}
