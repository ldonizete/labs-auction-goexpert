package auction

import (
	"context"
	"fullcycle-auction_go/configuration/logger"
	"fullcycle-auction_go/internal/entity/auction_entity"
	"fullcycle-auction_go/internal/internal_error"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	defaultAuctionDuration = time.Minute * 5
	closeAuctionTimeout    = time.Second * 10
)

type AuctionEntityMongo struct {
	Id          string                          `bson:"_id"`
	ProductName string                          `bson:"product_name"`
	Category    string                          `bson:"category"`
	Description string                          `bson:"description"`
	Condition   auction_entity.ProductCondition `bson:"condition"`
	Status      auction_entity.AuctionStatus    `bson:"status"`
	Timestamp   int64                           `bson:"timestamp"`
}
type AuctionRepository struct {
	Collection      *mongo.Collection
	auctionDuration time.Duration
}

func NewAuctionRepository(database *mongo.Database) *AuctionRepository {
	return &AuctionRepository{
		Collection:      database.Collection("auctions"),
		auctionDuration: getAuctionDuration(),
	}
}

func (ar *AuctionRepository) CreateAuction(
	ctx context.Context,
	auctionEntity *auction_entity.Auction) *internal_error.InternalError {
	auctionEntityMongo := &AuctionEntityMongo{
		Id:          auctionEntity.Id,
		ProductName: auctionEntity.ProductName,
		Category:    auctionEntity.Category,
		Description: auctionEntity.Description,
		Condition:   auctionEntity.Condition,
		Status:      auctionEntity.Status,
		Timestamp:   auctionEntity.Timestamp.Unix(),
	}
	_, err := ar.Collection.InsertOne(ctx, auctionEntityMongo)
	if err != nil {
		logger.Error("Error trying to insert auction", err)
		return internal_error.NewInternalServerError("Error trying to insert auction")
	}

	go ar.closeAuctionAfter(auctionEntity.Id, auctionEntity.Timestamp.Add(ar.auctionDuration))

	return nil
}

// closeAuctionAfter blocks until endTime and then marks the auction as
// Completed. It runs in its own goroutine and therefore must not use the
// request context, which is cancelled as soon as the HTTP handler returns.
func (ar *AuctionRepository) closeAuctionAfter(auctionId string, endTime time.Time) {
	timer := time.NewTimer(time.Until(endTime))
	defer timer.Stop()
	<-timer.C

	ctx, cancel := context.WithTimeout(context.Background(), closeAuctionTimeout)
	defer cancel()

	// Filtering by Active keeps the update idempotent: an auction that is
	// already Completed is left untouched.
	filter := bson.M{"_id": auctionId, "status": auction_entity.Active}
	update := bson.M{"$set": bson.M{"status": auction_entity.Completed}}
	if _, err := ar.Collection.UpdateOne(ctx, filter, update); err != nil {
		logger.Error("Error trying to close auction", err)
	}
}

// getAuctionDuration reads AUCTION_DURATION and falls back to
// AUCTION_INTERVAL, the variable the bid repository uses to decide whether an
// auction is still open, so both always agree on when an auction ends.
func getAuctionDuration() time.Duration {
	for _, name := range []string{"AUCTION_DURATION", "AUCTION_INTERVAL"} {
		duration, err := time.ParseDuration(os.Getenv(name))
		if err == nil && duration > 0 {
			return duration
		}
	}

	return defaultAuctionDuration
}
