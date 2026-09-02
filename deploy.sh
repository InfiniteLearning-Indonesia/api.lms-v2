#!/bin/bash

# Exit script immediately on any error
set -e

echo "🚀 Starting Automated Deployment for LMS Backend..."

echo "1/7 🔄 Fetching and pulling latest changes from Git staging..."
git fetch origin staging
git pull origin staging

echo "2/7 🧹 Cleaning old build directory..."
rm -rf dist

echo "3/7 🔨 Building NestJS application..."
npm run build

echo "4/7 🔄 Restarting PM2 process..."
export NODE_ENV=production

if pm2 list | grep -q "backend-lms"; then
    pm2 delete "backend-lms"
fi

pm2 start dist/src/main.js --name "backend-lms" --env production || pm2 start dist/main.js --name "backend-lms" --env production
pm2 save

echo "5/7 🛡️ Testing Nginx configuration..."
sudo nginx -t

echo "6/7 🔄 Reloading Nginx service..."
sudo systemctl reload nginx

echo "7/7 📊 Current PM2 Status:"
pm2 status

echo "✅ DEPLOYMENT COMPLETED SUCCESSFULLY!"
echo "💡 Run 'pm2 logs backend-lms' to view real-time diagnostic logs."
