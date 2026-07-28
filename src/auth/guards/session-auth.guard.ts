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
    console.log('[AUTH DIAGNOSTIC] Cookie Header:', request.headers.cookie || 'NO COOKIE RECEIVED');
    console.log('[AUTH DIAGNOSTIC] Current Session ID:', request.sessionID);

    let userId = request.session?.userId;

    // Fallback: Check if Token is passed in Authorization header or Query param
    let token = request.query?.token || request.query?.sid;
    const authHeader = request.headers.authorization;
    if (authHeader && authHeader.startsWith('Bearer ')) {
      token = authHeader.substring(7);
    }

    if (!userId && token && request.sessionStore) {
      console.log(`[AUTH DIAGNOSTIC FALLBACK] Looking up token/sessionID "${token}" in sessionStore...`);
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
        console.log(`[AUTH DIAGNOSTIC FALLBACK SUCCESS] Found userId "${userId}" from Session Store using token!`);
      } else {
        console.warn(`[AUTH DIAGNOSTIC FALLBACK FAIL] Token "${token}" not found in sessionStore.`);
      }
    }

    if (!userId) {
      console.error(
        `[AUTH DIAGNOSTIC REJECT 401] No userId in session or token! URL: ${request.url}. Cookies received: "${request.headers.cookie || 'none'}"`
      );
      console.log('================ [AUTH DIAGNOSTIC END] ================');
      throw new UnauthorizedException('Sesi login berakhir atau belum terautentikasi.');
    }

    try {
      const user = await this.usersService.findById(userId);
      if (!user) {
        console.error(
          `[AUTH DIAGNOSTIC REJECT 401] User ID ${userId} not found in DB!`
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
