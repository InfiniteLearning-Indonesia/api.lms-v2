import { Injectable, NestMiddleware, Logger } from '@nestjs/common';
import { Request, Response, NextFunction } from 'express';
import * as fs from 'fs';
import * as path from 'path';

@Injectable()
export class HttpLoggerMiddleware implements NestMiddleware {
  private readonly logger = new Logger('HTTP');
  private readonly LOGS_DIR = path.join(process.cwd(), 'logs');

  private getLogFile(): string {
    const now = new Date();
    const yyyy = now.getFullYear();
    const mm = String(now.getMonth() + 1).padStart(2, '0');
    return path.join(this.LOGS_DIR, `http-requests-${yyyy}-${mm}.log`);
  }

  private ensureLogsDir() {
    if (!fs.existsSync(this.LOGS_DIR)) {
      fs.mkdirSync(this.LOGS_DIR, { recursive: true });
    }
  }

  use(req: Request, res: Response, next: NextFunction): void {
    const startAt = process.hrtime();
    const { method, originalUrl, ip } = req;

    // Skip internal health-check polling to avoid log spam
    const isHealthPoll = originalUrl === '/health' && method === 'GET';

    res.on('finish', () => {
      const diff = process.hrtime(startAt);
      const ms = Math.round(diff[0] * 1000 + diff[1] / 1e6);
      const { statusCode } = res;

      const timestampWib = new Date().toLocaleString('id-ID', {
        timeZone: 'Asia/Jakarta',
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false,
      });

      const logLine = `[${timestampWib}] ${method.padEnd(6)} ${String(statusCode)} ${ms}ms  ${originalUrl}  ip=${ip ?? '-'}`;

      // Always log to pm2 console (except health poll)
      if (!isHealthPoll) {
        if (statusCode >= 500) {
          this.logger.error(logLine);
        } else if (statusCode >= 400) {
          this.logger.warn(logLine);
        } else {
          this.logger.log(logLine);
        }

        // Non-blocking file append
        setImmediate(() => {
          try {
            this.ensureLogsDir();
            fs.appendFileSync(this.getLogFile(), logLine + '\n', 'utf8');
          } catch (_) {
            // Silently ignore file write errors — never block the request
          }
        });
      }
    });

    next();
  }
}
