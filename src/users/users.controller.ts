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
@Roles('admin')
export class UsersController {
  constructor(private readonly usersService: UsersService) {}

  @Post()
  invite(@Body() dto: CreateUserDto) {
    console.log(
      '[USERS CONTROLLER] POST /users called with:',
      JSON.stringify(dto),
    );
    return this.usersService.invite(dto);
  }

  @Post('invite')
  inviteAlias(@Body() dto: CreateUserDto) {
    console.log(
      '[USERS CONTROLLER] POST /users/invite called with:',
      JSON.stringify(dto),
    );
    return this.usersService.invite(dto);
  }

  @Post('bulk')
  bulkInvite(@Body() dto: BulkInviteDto) {
    return this.usersService.bulkInvite(dto);
  }

  @Post('bulk-import-csv')
  bulkInviteAlias(@Body() dto: BulkInviteDto) {
    return this.usersService.bulkInvite(dto);
  }

  @Delete('bulk-delete')
  bulkDeleteDelete(@Body() body: { ids?: string[]; userIds?: string[] }) {
    const ids = body?.ids || body?.userIds || [];
    return this.usersService.bulkDelete(ids);
  }

  @Post('bulk-delete')
  bulkDeletePost(@Body() body: { ids?: string[]; userIds?: string[] }) {
    const ids = body?.ids || body?.userIds || [];
    return this.usersService.bulkDelete(ids);
  }

  @Get()
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
  findOne(@Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.findById(id);
  }

  @Patch(':id/suspend')
  @Roles('admin', 'mentor')
  suspend(@Req() req: any, @Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.suspend(id, req.user);
  }

  @Patch(':id/unsuspend')
  @Roles('admin', 'mentor')
  unsuspend(@Req() req: any, @Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.unsuspend(id, req.user);
  }

  @Patch(':id/email')
  updateEmail(
    @Req() req: any,
    @Param('id', ParseUUIDPipe) id: string,
    @Body('email') newEmail: string,
  ) {
    return this.usersService.updateEmail(id, newEmail, req.user);
  }

  @Patch(':id')
  async update(
    @Req() req: any,
    @Param('id', ParseUUIDPipe) id: string,
    @Body() dto: UpdateUserDto,
  ) {
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
  sendWarningEmail(@Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.sendWarningEmail(id);
  }

  @Delete(':id')
  remove(@Req() req: any, @Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.remove(id, req.user);
  }
}
