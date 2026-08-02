import { Controller, Get, Query } from '@nestjs/common';
import { HealthHistoryService, DayUptime } from './health-history.service.js';
import { DataSource } from 'typeorm';

@Controller('health')
export class HealthController {
  constructor(
    private readonly healthHistoryService: HealthHistoryService,
    private readonly dataSource: DataSource,
  ) {}

  @Get()
  async check() {
    let dbStatus = 'disconnected';
    let dbLatencyMs = 0;
    
    if (this.dataSource.isInitialized) {
      const start = process.hrtime();
      try {
        await this.dataSource.query('SELECT 1');
        const diff = process.hrtime(start);
        dbLatencyMs = Math.round(diff[0] * 1000 + diff[1] / 1e6);
        dbStatus = 'connected';
      } catch (err) {
        dbStatus = 'error';
      }
    }

    const memUsage = process.memoryUsage();
    
    return {
      status: dbStatus === 'connected' && dbLatencyMs < 1500 ? 'ok' : 'degraded',
      timestamp: new Date().toLocaleString('id-ID', { timeZone: 'Asia/Jakarta' }),
      uptimeSeconds: process.uptime(),
      database: {
        status: dbStatus,
        latencyMs: dbLatencyMs,
      },
      memory: {
        rssMb: Math.round(memUsage.rss / 1024 / 1024),
        heapTotalMb: Math.round(memUsage.heapTotal / 1024 / 1024),
        heapUsedMb: Math.round(memUsage.heapUsed / 1024 / 1024),
      }
    };
  }

  @Get('history')
  getHistory() {
    return this.healthHistoryService.getHistory();
  }

  @Get('daily')
  getDaily(@Query('days') days?: string): DayUptime[] {
    const daysNum = Math.min(parseInt(days || '90', 10) || 90, 180);
    return this.healthHistoryService.getDailyUptime(daysNum);
  }
}

