package server

import (
	"context"
	"errors"
	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/docs"
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/gofiber/contrib/otelfiber/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/swagger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	slogfiber "github.com/samber/slog-fiber"
	"log/slog"
	"time"
)

func initGlobalMiddlewares(app *fiber.App, cfg *config.Config) {
	app.Use(
		compress.New(compress.Config{
			Level: compress.LevelDefault,
		}),

		slogfiber.NewWithFilters(slog.Default(), slogfiber.IgnorePath("/health")),

		cors.New(cors.Config{
			AllowOrigins: "*", // TODO - add allowed origins
			AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Request-ID",
			AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
		}),

		favicon.New(),
		limiter.New(limiter.Config{
			Max:               cfg.RateLimitMax,
			Expiration:        time.Duration(cfg.RateLimitWindow) * time.Second,
			LimiterMiddleware: limiter.SlidingWindow{},
		}),
	)

	app.Use(otelfiber.Middleware())
	app.Use(func(c *fiber.Ctx) error {

		//c.Set("X-Request-ID", c.Locals("requestID").(string))
		return c.Next()
	})

}

func registerHttpRoutes(app *fiber.App, cfg *config.Config, db postgres.DB) {
	// swagger
	docs.SwaggerInfo.Version = "1.0.0"
	app.Get("/swagger/*", swagger.HandlerDefault)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	app.Static("/", "./public")

	apiRoutes := app.Group("/v1")

	apiRoutes.Get("/test", withTransaction(db, func(c *fiber.Ctx, tx pgx.Tx) error {
		// Example of using the transaction
		id := uuid.New()

		// Perform some database operations using the transaction
		if _, err := tx.Exec(
			c.UserContext(),
			"INSERT INTO test_table (id, created_at, updated_at, text) VALUES ($1, $2, $3, $4)",
			id, time.Now(), time.Now(), "sdssdssdsdsdssdsd"); err != nil {
			slog.Error("failed to insert into test_table", slog.String("error", err.Error()))
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		// get value from db
		row, _ := tx.Query(c.UserContext(), "SELECT id, text FROM test_table")
		defer row.Close()
		// return all rows
		rows := make([]Resp, 0)
		for row.Next() {
			var id string
			var text string
			if err := row.Scan(&id, &text); err != nil {
				slog.Error("failed to scan row", slog.String("error", err.Error()))
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			rows = append(rows, Resp{ID: id, Text: text})
		}

		return c.JSON(rows)
	}))

}

type Resp struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type withTransactionHandler func(c *fiber.Ctx, tx pgx.Tx) error

func withTransaction(db postgres.DB, handler withTransactionHandler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()

		tx, err := db.Begin(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fiber.ErrRequestTimeout
			}

			return err
		}

		err = handler(c, tx)
		ctx, cancel = context.WithTimeout(c.UserContext(), 1*time.Second)
		defer cancel()
		if err != nil || c.Response().StatusCode() >= 400 {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				slog.Error("failed to rollback transaction", slog.String("error", rollbackErr.Error()))
			}
		} else {
			if commitErr := tx.Commit(ctx); commitErr != nil {
				slog.Error("failed to commit transaction", slog.String("error", commitErr.Error()))
			}
		}

		return err
	}
}
