import { Module } from '@nestjs/common';
import { HealthController } from './health.controller.js';
import { HealthHistoryService } from './health-history.service.js';

@Module({
  controllers: [HealthController],
  providers: [HealthHistoryService],
})
export class HealthModule {}
