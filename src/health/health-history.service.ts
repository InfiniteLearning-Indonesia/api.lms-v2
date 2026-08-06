import {
  Injectable,
  Logger,
  OnModuleInit,
  OnModuleDestroy,
} from '@nestjs/common';
import { DataSource } from 'typeorm';
import * as fs from 'fs';
import * as path from 'path';

export interface HealthSnapshot {
  timestamp: string;
  status: 'ok' | 'degraded' | 'down';
  uptimeMs: number;
  memoryUsageMb: number;
  dbLatencyMs: number;
}

@Injectable()
export class HealthHistoryService implements OnModuleInit, OnModuleDestroy {
  private readonly logger = new Logger(HealthHistoryService.name);
  private intervalId: NodeJS.Timeout;
  private readonly HISTORY_FILE = path.join(
    process.cwd(),
    'status-history.json',
  );
  private readonly MAX_HISTORY_DAYS = 7;

  constructor(private readonly dataSource: DataSource) {}

  onModuleInit() {
    this.ensureFileExists();
    // Record snapshot every 15 minutes
    this.intervalId = setInterval(() => this.recordSnapshot(), 15 * 60 * 1000);
    // Trigger one immediately
    setTimeout(() => this.recordSnapshot(), 5000);
  }

  onModuleDestroy() {
    if (this.intervalId) {
      clearInterval(this.intervalId);
    }
  }

  private ensureFileExists() {
    if (!fs.existsSync(this.HISTORY_FILE)) {
      fs.writeFileSync(this.HISTORY_FILE, JSON.stringify([]), 'utf-8');
    }
  }

  private async measureDbLatency(): Promise<{
    isConnected: boolean;
    latencyMs: number;
  }> {
    if (!this.dataSource.isInitialized) {
      return { isConnected: false, latencyMs: 0 };
    }
    const start = process.hrtime();
    try {
      await this.dataSource.query('SELECT 1');
      const diff = process.hrtime(start);
      const latencyMs = diff[0] * 1000 + diff[1] / 1e6;
      return { isConnected: true, latencyMs: Math.round(latencyMs) };
    } catch (err) {
      return { isConnected: false, latencyMs: 0 };
    }
  }

  async recordSnapshot() {
    try {
      const dbStats = await this.measureDbLatency();
      const memUsage = process.memoryUsage();
      const memoryMb = Math.round(memUsage.rss / 1024 / 1024);

      const status =
        dbStats.isConnected && dbStats.latencyMs < 1500
          ? 'ok'
          : dbStats.isConnected
            ? 'degraded'
            : 'down';

      // Use WIB format for timestamp
      const now = new Date();
      const timestampWib = now.toLocaleString('id-ID', {
        timeZone: 'Asia/Jakarta',
      });

      const snapshot: HealthSnapshot = {
        timestamp: timestampWib,
        status,
        uptimeMs: Math.round(process.uptime() * 1000),
        memoryUsageMb: memoryMb,
        dbLatencyMs: dbStats.latencyMs,
      };

      let history: HealthSnapshot[] = [];
      if (fs.existsSync(this.HISTORY_FILE)) {
        try {
          const content = fs.readFileSync(this.HISTORY_FILE, 'utf-8');
          if (content.trim()) {
            history = JSON.parse(content);
          }
        } catch (e) {
          this.logger.error('Failed to parse status-history.json', e);
        }
      }

      history.push(snapshot);

      // Calculate cutoff time (7 days ago in local time roughly)
      const cutoffTime =
        now.getTime() - this.MAX_HISTORY_DAYS * 24 * 60 * 60 * 1000;

      // Keep max 96 snapshots (24 hours * 4 per hour) to keep the file tiny
      // Wait, 15m interval * 4 = 4/hr * 24 = 96/day. Let's keep 7 days = 672 items max.
      if (history.length > 700) {
        history = history.slice(history.length - 700);
      }

      fs.writeFileSync(
        this.HISTORY_FILE,
        JSON.stringify(history, null, 2),
        'utf-8',
      );
    } catch (err) {
      this.logger.error('Failed to record health snapshot', err);
    }
  }

  getHistory(): HealthSnapshot[] {
    try {
      if (fs.existsSync(this.HISTORY_FILE)) {
        const content = fs.readFileSync(this.HISTORY_FILE, 'utf-8');
        return JSON.parse(content);
      }
    } catch (e) {
      this.logger.error('Failed to read status-history.json', e);
    }
    return [];
  }

  getDailyUptime(days: number = 90): DayUptime[] {
    const history = this.getHistory();
    const result: DayUptime[] = [];

    const today = new Date();

    for (let i = days - 1; i >= 0; i--) {
      const targetDate = new Date(today);
      targetDate.setDate(today.getDate() - i);

      // Build date key in WIB format "DD/MM/YYYY"
      const day = String(targetDate.getDate()).padStart(2, '0');
      const month = String(targetDate.getMonth() + 1).padStart(2, '0');
      const year = targetDate.getFullYear();
      const dateKey = `${day}/${month}/${year}`;

      // Match snapshots that belong to this date
      // Snapshots have timestampWib like "2/8/2026, 06.13.00"
      const daySnapshots = history.filter((s) => {
        // Parse "D/M/YYYY, HH.mm.ss" — Indonesian locale from toLocaleString
        const parts = s.timestamp.split(',')[0]?.split('/');
        if (!parts || parts.length < 3) return false;
        const sDay = parts[0].trim().padStart(2, '0');
        const sMonth = parts[1].trim().padStart(2, '0');
        const sYear = parts[2].trim();
        return `${sDay}/${sMonth}/${sYear}` === dateKey;
      });

      if (daySnapshots.length === 0) {
        result.push({
          date: dateKey,
          uptimePercent: null,
          status: 'no-data',
          incident: null,
        });
        continue;
      }

      const okCount = daySnapshots.filter((s) => s.status === 'ok').length;
      const degradedSnapshots = daySnapshots.filter(
        (s) => s.status === 'degraded',
      );
      const downSnapshots = daySnapshots.filter((s) => s.status === 'down');
      const uptimePercent =
        Math.round((okCount / daySnapshots.length) * 1000) / 10;

      let status: 'operational' | 'degraded' | 'outage';
      if (uptimePercent >= 99) status = 'operational';
      else if (uptimePercent >= 70) status = 'degraded';
      else status = 'outage';

      let incident: string | null = null;
      if (degradedSnapshots.length > 0 || downSnapshots.length > 0) {
        const maxLatency = Math.max(...daySnapshots.map((s) => s.dbLatencyMs));
        const incidentParts: string[] = [];
        if (downSnapshots.length > 0) {
          incidentParts.push(
            `${downSnapshots.length} snapshot down (koneksi DB gagal)`,
          );
        }
        if (degradedSnapshots.length > 0) {
          incidentParts.push(`${degradedSnapshots.length} snapshot degraded`);
        }
        if (maxLatency > 0) {
          incidentParts.push(`latency DB maks: ${maxLatency}ms`);
        }
        incidentParts.push(`total snapshot: ${daySnapshots.length}`);
        incident = incidentParts.join(' — ');
      }

      result.push({ date: dateKey, uptimePercent, status, incident });
    }

    return result;
  }
}

export interface DayUptime {
  date: string;
  uptimePercent: number | null;
  status: 'operational' | 'degraded' | 'outage' | 'no-data';
  incident: string | null;
}
