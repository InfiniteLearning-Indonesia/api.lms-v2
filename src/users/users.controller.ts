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
    return this.usersService.invite(dto);
  }

  @Post('invite')
  inviteAlias(@Body() dto: CreateUserDto) {
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
      console.log(`[USERS DIAGNOSTIC SUCCESS] GET /users returned ${result?.length || 0} user records.`);
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
  suspend(@Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.suspend(id);
  }

  @Patch(':id/unsuspend')
  @Roles('admin', 'mentor')
  unsuspend(@Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.unsuspend(id);
  }

  @Patch(':id/email')
  updateEmail(
    @Param('id', ParseUUIDPipe) id: string,
    @Body('email') newEmail: string,
  ) {
    return this.usersService.updateEmail(id, newEmail);
  }

  @Patch(':id')
  update(
    @Param('id', ParseUUIDPipe) id: string,
    @Body() dto: UpdateUserDto,
  ) {
    return this.usersService.update(id, dto);
  }

  @Post(':id/send-warning-email')
  sendWarningEmail(@Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.sendWarningEmail(id);
  }

  @Delete(':id')
  remove(@Param('id', ParseUUIDPipe) id: string) {
    return this.usersService.remove(id);
  }
}
