import {
  CanActivate,
  ExecutionContext,
  Injectable,
  ForbiddenException,
} from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { UsersService } from '../../users/users.service.js';
import { UserStatus } from '../../users/entities/user.entity.js';

@Injectable()
export class RolesGuard implements CanActivate {
  constructor(
    private readonly reflector: Reflector,
    private readonly usersService: UsersService,
  ) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const requiredRoles = this.reflector.getAllAndOverride<string[]>('roles', [
      context.getHandler(),
      context.getClass(),
    ]);
    if (!requiredRoles) {
      return true;
    }

    const request = context.switchToHttp().getRequest();

    // Fallback: if SessionAuthGuard did not attach request.user, load it using session userId
    if (!request.user && request.session?.userId) {
      const user = await this.usersService.findById(request.session.userId);
      request.user = user;
    }

    const user = request.user;
    if (
      !user ||
      !user.roles ||
      !requiredRoles.some((role) => user.roles.includes(role))
    ) {
      throw new ForbiddenException(
        'Anda tidak memiliki izin untuk mengakses resource ini.',
      );
    }

    if (user.status === UserStatus.SUSPENDED) {
      throw new ForbiddenException(
        'Akun kamu telah di-suspend oleh Admin. Akses ditolak.',
      );
    }

    return true;
  }
}
