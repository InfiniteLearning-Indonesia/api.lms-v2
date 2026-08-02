import { Controller, Get, Query, UseGuards, UnauthorizedException } from '@nestjs/common';
import { AuditService } from './audit.service.js';
import { SessionAuthGuard } from '../auth/guards/session-auth.guard.js';
import { RolesGuard } from '../auth/guards/roles.guard.js';
import { Roles } from '../auth/decorators/roles.decorator.js';

@Controller('audit')
@UseGuards(SessionAuthGuard, RolesGuard)
export class AuditController {
  constructor(private readonly auditService: AuditService) {}

  @Get()
  @Roles('admin')
  async getLogs(
    @Query('page') page: string,
    @Query('limit') limit: string,
    @Query('level') level?: string,
    @Query('category') category?: string,
  ) {
    const pageNum = parseInt(page, 10) || 1;
    const limitNum = parseInt(limit, 10) || 20;
    
    return this.auditService.getLogs(pageNum, limitNum, level, category);
  }
}
