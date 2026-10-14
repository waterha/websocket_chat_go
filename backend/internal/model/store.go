package model

import "github.com/jackc/pgx/v5/pgxpool"

type Store struct{ DB *pgxpool.Pool }
