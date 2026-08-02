import { Injectable, Logger, OnModuleInit } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository, LessThan } from 'typeorm';
import { AuditLog } from './entities/audit-log.entity.js';
import * as fs from 'fs';
import * as path from 'path';

export interface CreateLogDto {
  level?: 'INFO' | 'WARN' | 'ERROR';
  category: 'SECURITY' | 'AUTH' | 'MUTATION' | 'SYSTEM';
  userId?: string;
  userEmail?: string;
  userRole?: string;
  ipAddress?: string;
  action: string;
  method?: string;
  path?: string;
  statusCode?: number;
  details?: any;
}

@Injectable()
export class AuditService implements OnModuleInit {
  private readonly logger = new Logger(AuditService.name);
  private readonly LOGS_DIR = path.join(process.cwd(), 'logs');

  constructor(
    @InjectRepository(AuditLog)
    private auditRepository: Repository<AuditLog>,
  ) {}

  onModuleInit() {
    if (!fs.existsSync(this.LOGS_DIR)) {
      fs.mkdirSync(this.LOGS_DIR, { recursive: true });
    }
    // Run pruning job daily at startup and via interval
    this.archiveOldLogs();
    setInterval(() => this.archiveOldLogs(), 24 * 60 * 60 * 1000);
  }

  // Non-blocking fire-and-forget logger
  async logEvent(dto: CreateLogDto) {
    setImmediate(async () => {
      try {
        const now = new Date();
        const timestampWib = now.toLocaleString('id-ID', { timeZone: 'Asia/Jakarta' });
        
        const log = this.auditRepository.create({
          ...dto,
          level: dto.level || 'INFO',
          timestampWib,
        });
        await this.auditRepository.save(log);
      } catch (err) {
        this.logger.error('Failed to save audit log', err);
      }
    });
  }

  async getLogs(page: number = 1, limit: number = 20, level?: string, category?: string) {
    const query = this.auditRepository.createQueryBuilder('log')
      .orderBy('log.createdAt', 'DESC')
      .skip((page - 1) * limit)
      .take(limit);

    if (level && level !== 'all') {
      query.andWhere('log.level = :level', { level });
    }
    if (category && category !== 'all') {
      query.andWhere('log.category = :category', { category });
    }

    const [items, total] = await query.getManyAndCount();
    return {
      data: items,
      total,
      page,
      limit,
      totalPages: Math.ceil(total / limit)
    };
  }

  // Archive logs older than 30 days to local JSONL and delete from DB
  private async archiveOldLogs() {
    try {
      const thirtyDaysAgo = new Date();
      thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30);

      const oldLogs = await this.auditRepository.find({
        where: { createdAt: LessThan(thirtyDaysAgo) },
        order: { createdAt: 'ASC' }
      });

      if (oldLogs.length === 0) return;

      const groupedByMonth: Record<string, AuditLog[]> = {};
      
      oldLogs.forEach(log => {
        const date = new Date(log.createdAt);
        const yyyy = date.getFullYear();
        const mm = String(date.getMonth() + 1).padStart(2, '0');
        const key = `${yyyy}-${mm}`;
        if (!groupedByMonth[key]) groupedByMonth[key] = [];
        groupedByMonth[key].push(log);
      });

      for (const [monthKey, logs] of Object.entries(groupedByMonth)) {
        const filename = path.join(this.LOGS_DIR, `audit-${monthKey}.jsonl`);
        const lines = logs.map(l => JSON.stringify(l)).join('\n') + '\n';
        fs.appendFileSync(filename, lines, 'utf8');
      }

      // After successful file appending, delete from DB
      const idsToDelete = oldLogs.map(l => l.id);
      
      // Delete in chunks of 500 to prevent locking
      for (let i = 0; i < idsToDelete.length; i += 500) {
        const chunk = idsToDelete.slice(i, i + 500);
        await this.auditRepository.delete(chunk);
      }
      
      this.logger.log(`Archived ${oldLogs.length} old audit logs to JSONL files.`);
    } catch (err) {
      this.logger.error('Failed to archive old audit logs', err);
    }
  }
}
