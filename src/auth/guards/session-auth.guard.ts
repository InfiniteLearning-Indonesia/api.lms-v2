import {
  CanActivate,
  ExecutionContext,
  Injectable,
  UnauthorizedException,
  ForbiddenException,
  Logger,
} from '@nestjs/common';
import { UsersService } from '../../users/users.service.js';
import { UserStatus } from '../../users/entities/user.entity.js';

@Injectable()
export class SessionAuthGuard implements CanActivate {
  private readonly logger = new Logger(SessionAuthGuard.name);
  constructor(private readonly usersService: UsersService) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest();

    let userId = request.session?.userId;

    // Fallback: Check if Token is passed in Authorization header or Query param
    let token = request.query?.token || request.query?.sid;
    const authHeader = request.headers.authorization;
    if (authHeader && authHeader.startsWith('Bearer ')) {
      token = authHeader.substring(7);
    }

    if (!userId && token && request.sessionStore) {
      const storeSession: any = await new Promise((resolve) => {
        request.sessionStore.get(token, (err: any, sess: any) => {
          if (err || !sess) resolve(null);
          else resolve(sess);
        });
      });

      if (storeSession && storeSession.userId) {
        userId = storeSession.userId;
        request.session.userId = userId;
        request.session.userEmail = storeSession.userEmail;
        if (typeof request.session.save === 'function') {
          request.session.save();
        }
      }
    }

    if (!userId) {
      throw new UnauthorizedException(
        'Sesi login berakhir atau belum terautentikasi.',
      );
    }

    try {
      const user = await this.usersService.findById(userId);
      if (!user) {
        throw new UnauthorizedException(
          'User session is invalid or user not found in DB.',
        );
      }

      if (user.status === UserStatus.SUSPENDED) {
        throw new ForbiddenException(
          'Akun kamu telah di-suspend oleh Admin. Akses ditolak.',
        );
      }

      request.user = user;
      return true;
    } catch (err: any) {
      this.logger.error(`Auth error for userId ${userId}: ${err.message}`);
      throw err;
    }
  }
}
