# Python Data Engineering

Modern data processing with Polars and pipeline patterns.

## Polars Basics

```python
import polars as pl

# Lazy evaluation (scan_*)
df = pl.scan_parquet("data/*.parquet")

# DataFrame operations
result = (
    df
    .filter(pl.col("status") == "active")
    .select([
        "user_id",
        "email",
        pl.col("created_at").cast(pl.Date).alias("date"),
        (pl.col("amount") * 1.1).alias("amount_with_tax"),
    ])
    .group_by("date")
    .agg([
        pl.count().alias("count"),
        pl.sum("amount_with_tax").alias("total_amount"),
    ])
    .sort("date", descending=True)
    .collect()  # Execute lazy query
)
```

## Data Transformations

```python
def transform_orders(df: pl.LazyFrame) -> pl.LazyFrame:
    return (
        df
        .with_columns([
            pl.col("order_date").str.strptime(pl.Datetime, "%Y-%m-%d %H:%M:%S"),
            (pl.col("quantity") * pl.col("unit_price")).alias("line_total"),
        ])
        .with_columns([
            pl.col("discount").fill_null(0.0),
        ])
        .filter((pl.col("quantity") > 0) & (pl.col("unit_price") > 0))
    )

# Window functions
df_with_rank = df.with_columns([
    pl.col("amount").rank(descending=True).over("category").alias("rank_in_category"),
    pl.col("amount").sum().over("category").alias("category_total"),
])
```

## Schema Validation

```python
from pydantic import BaseModel, Field
import polars as pl

class OrderSchema(BaseModel):
    order_id: int = Field(gt=0)
    user_id: int = Field(gt=0)
    amount: float = Field(gt=0)
    status: str = Field(pattern="^(pending|completed|cancelled)$")

# Define schema in Polars
ORDER_SCHEMA = {
    "order_id": pl.Int64,
    "user_id": pl.Int64,
    "amount": pl.Float64,
    "status": pl.Utf8,
}

df = pl.scan_csv("orders.csv", schema=ORDER_SCHEMA)
```

## Streaming Processing

```python
from collections.abc import Iterator

def process_in_batches(file_path: Path, batch_size: int = 10_000) -> Iterator[pl.DataFrame]:
    reader = pl.read_csv_batched(file_path, batch_size=batch_size)
    while True:
        batch = reader.next_batches(1)
        if not batch:
            break
        yield transform_batch(batch[0])

# Lazy streaming to file
result = (
    pl.scan_parquet("large_dataset/*.parquet")
    .filter(pl.col("year") == 2024)
    .group_by("user_id")
    .agg([pl.sum("amount").alias("total")])
    .sink_parquet("output.parquet")
)
```

## Time Series Operations

```python
timeseries = (
    df
    .sort("timestamp")
    .group_by_dynamic("timestamp", every="1h")
    .agg([
        pl.count().alias("count"),
        pl.mean("value").alias("avg_value"),
    ])
)

# Rolling window
rolling = df.with_columns([
    pl.col("value").rolling_mean(window_size=7).alias("rolling_7d_avg"),
])
```

## ETL Pipeline Pattern

```python
from typing import Protocol

class DataSource(Protocol):
    def read(self) -> pl.LazyFrame: ...

class DataSink(Protocol):
    def write(self, df: pl.DataFrame) -> None: ...

class ETLPipeline:
    def __init__(self, source: DataSource, sink: DataSink):
        self.source = source
        self.sink = sink
        self.transformations = []

    def add_transform(self, func):
        self.transformations.append(func)
        return self

    def run(self):
        df = self.source.read()
        for transform in self.transformations:
            df = transform(df)
        self.sink.write(df.collect())

# Usage
pipeline = (
    ETLPipeline(ParquetSource("input/"), ParquetSink("output/"))
    .add_transform(clean_data)
    .add_transform(enrich_data)
)
pipeline.run()
```

## Critical Rules

### Always
- Use lazy evaluation (scan_*) for large datasets
- Define schemas explicitly
- Use streaming for memory efficiency
- Prefer Polars over Pandas for new projects

### Never
- Load entire dataset unnecessarily
- Skip schema validation
- Use iterrows() (use iter_rows())
- Forget to collect() lazy frames
