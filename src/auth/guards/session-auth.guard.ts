import {
  CanActivate,
  ExecutionContext,
  Injectable,
  UnauthorizedException,
} from '@nestjs/common';
import { UsersService } from '../../users/users.service.js';

@Injectable()
export class SessionAuthGuard implements CanActivate {
  constructor(private readonly usersService: UsersService) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest();
    
    console.log('================ [AUTH DIAGNOSTIC START] ================');
    console.log('[AUTH DIAGNOSTIC] Path:', request.method, request.url);
    console.log('[AUTH DIAGNOSTIC] Origin Header:', request.headers.origin || request.headers.referer || 'none');
    console.log('[AUTH DIAGNOSTIC] Cookie Header:', request.headers.cookie || 'NO COOKIE RECEIVED');
    console.log('[AUTH DIAGNOSTIC] Session ID:', request.sessionID);
    console.log('[AUTH DIAGNOSTIC] Session Data:', JSON.stringify(request.session || {}));

    if (!request.session?.userId) {
      console.error(
        `[AUTH DIAGNOSTIC REJECT 401] No userId in session! URL: ${request.url}. Cookies received: "${request.headers.cookie || 'none'}"`
      );
      console.log('================ [AUTH DIAGNOSTIC END] ================');
      throw new UnauthorizedException('Sesi login berakhir atau belum terautentikasi.');
    }

    try {
      const user = await this.usersService.findById(request.session.userId);
      if (!user) {
        console.error(
          `[AUTH DIAGNOSTIC REJECT 401] User ID ${request.session.userId} from session not found in DB!`
        );
        console.log('================ [AUTH DIAGNOSTIC END] ================');
        throw new UnauthorizedException('User session is invalid or user not found in DB.');
      }

      console.log(
        `[AUTH DIAGNOSTIC PASS] Authorized user: ${user.email} (${user.id}), roles: ${JSON.stringify(user.roles)}`
      );
      console.log('================ [AUTH DIAGNOSTIC END] ================');
      request.user = user;
      return true;
    } catch (err: any) {
      console.error('[AUTH DIAGNOSTIC DB ERROR]', err);
      console.log('================ [AUTH DIAGNOSTIC END] ================');
      throw err;
    }
  }
}
