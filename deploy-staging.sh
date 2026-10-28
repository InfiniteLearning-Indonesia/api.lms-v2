#!/bin/bash

# Exit script immediately on any error
set -e

echo "Starting LMS staging deployment..."

echo "1/5 Fetching and pulling origin/staging..."
git fetch origin staging && git pull --ff-only origin staging || exit 1

echo "2/5 Cleaning and building staging..."
rm -rf dist && npm run build || exit 1

echo "3/5 Restarting staging..."
pm2 restart backend-lms-staging --update-env

echo "4/5 Saving PM2 process list..."
pm2 save

echo "5/5 Current PM2 status:"
pm2 status

echo "Staging deployment completed."
