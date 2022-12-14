#  How to run

## Prereqs

Install serverless framework and golang.
```
brew install serverless golang
```

## Run locally

```
make && serverless invoke local --function validateBankAccount -d '{"body" : "{\"accountNumber\": \"12345678\"}"}'
```

## Deploy

```
serverless deploy
```
