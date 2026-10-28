import {
  Controller,
  Get,
  Post,
  Patch,
  Delete,
  Body,
  Param,
  UseGuards,
  ParseUUIDPipe,
  Req,
  ForbiddenException,
} from '@nestjs/common';
import { UsersService } from './users.service.js';
import { CreateUserDto } from './dto/create-user.dto.js';
import { UpdateUserDto } from './dto/update-user.dto.js';
import { BulkInviteDto } from './dto/bulk-invite.dto.js';
import { SessionAuthGuard } from '../auth/guards/session-auth.guard.js';
import { RolesGuard } from '../auth/guards/roles.guard.js';
import { Roles } from '../auth/decorators/roles.decorator.js';

@Controller('users')
@UseGuards(SessionAuthGuard, RolesGuard)
export class UsersController {
  constructor(private readonly usersService: UsersService) {}

  @Post()
  @Roles('admin')
  invite(@Body() dto: CreateUserDto) {
    console.log(
      '[USERS CONTROLLER] POST /users called with:',
      JSON.stringify(dto),
    );
    return this.usersService.invite(dto);
  }

  @Post('invite')
  @Roles('admin')
  inviteAlias(@Body() dto: CreateUserDto) {
    console.log(
      '[USERS CONTROLLER] POST /users/invite called with:',
      JSON.stringify(dto),
    );
    return this.usersService.invite(dto);
  }

  @Post('bulk')
  @Roles('admin')
  bulkInvite(@Body() dto: BulkInviteDto) {
    return this.usersService.bulkInvite(dto);
  }

  @Post('bulk-import-csv')
  @Roles('admin')
  bulkInviteAlias(@Body() dto: BulkInviteDto) {
    return this.usersService.bulkInvite(dto);
  }

  @Delete('bulk-delete')
  @Roles('admin')
  bulkDeleteDelete(@Body() body: { ids?: string[]; userIds?: string[] }) {
    const ids = body?.ids || body?.userIds || [];
    return this.usersService.bulkDelete(ids);
  }

  @Post('bulk-delete')
  @Roles('admin')
  bulkDeletePost(@Body() body: { ids?: string[]; userIds?: string[] }) {
    const ids = body?.ids || body?.userIds || [];
    return this.usersService.bulkDelete(ids);
  }

  @Get()
  @Roles('admin')
  async findAll() {
    console.log('[USERS DIAGNOSTIC] GET /users called by admin');
    try {
      const result = await this.usersService.findAll();
      console.log(
        `[USERS DIAGNOSTIC SUCCESS] GET /users returned ${result?.length || 0} user records.`,
      );
      return result;
    } catch (err: any) {
      console.error('[USERS DIAGNOSTIC ERROR] GET /users error:', err);
      throw err;
    }
  }

  @Get(':id')
  findOne(@Req() req: any, @Param('id', ParseUUIDPipe) id: string) {
    const isAdmin = req.user?.roles?.includes('admin');
    if (!isAdmin && req.user?.id !== id) {
      throw new ForbiddenException(
        'Anda hanya dapat mengakses profil Anda sendiri.',
      );
    }
    return this.usersService.findById(id);
  }

  @Patch(':id/suspend')
  @Roles('admin')
  suspend(@Req() req: any, @Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.suspend(id, req.user);
  }

  @Patch(':id/unsuspend')
  @Roles('admin')
  unsuspend(@Req() req: any, @Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.unsuspend(id, req.user);
  }

  @Patch(':id/email')
  updateEmail(
    @Req() req: any,
    @Param('id', ParseUUIDPipe) id: string,
    @Body('email') newEmail: string,
  ) {
    const isAdmin = req.user?.roles?.includes('admin');
    if (!isAdmin && req.user?.id !== id) {
      throw new ForbiddenException(
        'Anda hanya dapat memperbarui email Anda sendiri.',
      );
    }
    return this.usersService.updateEmail(id, newEmail, req.user);
  }

  @Patch(':id')
  async update(
    @Req() req: any,
    @Param('id', ParseUUIDPipe) id: string,
    @Body() dto: UpdateUserDto,
  ) {
    const isAdmin = req.user?.roles?.includes('admin');
    if (!isAdmin && req.user?.id !== id) {
      throw new ForbiddenException(
        'Anda hanya dapat memperbarui profil Anda sendiri.',
      );
    }
    console.log(
      `[USERS CONTROLLER] PATCH /users/${id} received! Payload:`,
      JSON.stringify(dto),
    );
    try {
      const res = await this.usersService.update(id, dto, req.user);
      console.log(
        `[USERS CONTROLLER SUCCESS] PATCH /users/${id} completed! User assignedBatchIds:`,
        res.assignedBatchIds,
      );
      return res;
    } catch (err) {
      console.error(`[USERS CONTROLLER ERROR] PATCH /users/${id} failed:`, err);
      throw err;
    }
  }

  @Post(':id/send-warning-email')
  @Roles('admin')
  sendWarningEmail(@Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.sendWarningEmail(id);
  }

  @Delete(':id')
  @Roles('admin')
  remove(@Req() req: any, @Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.remove(id, req.user);
  }
}
