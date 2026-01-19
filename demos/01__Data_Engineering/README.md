# Data Engineering Demo

This demo shows how to use Ekaya as a Data Engineer.

Starting with an empty database, you will instruct AI to download a dataset, create a well-formed star schema from the CSV data, then import it.

You will use this populated database as inputs to the other demos.

## Prerequisites

1. You have successfully launched Ekaya Engine either with the QuickStart docker image or locally via cloning this repo.
1. You have created an new project via the [../README.md] and have connected the Postgres datasource to an empty database.
1. You have Claude Code and are familiar with how to use it.

## Setup

Make sure Ekaya Engine is running:

```bash
curl -s http://localhost:3443/ping
```

```bash
cd demos/01__Data_Engineering/


```
