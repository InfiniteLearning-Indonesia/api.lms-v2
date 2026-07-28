import {
  Controller,
  Get,
  Post,
  Req,
  Res,
  UseGuards,
} from '@nestjs/common';
import type { Response } from 'express';
import { AuthService } from './auth.service.js';
import { GoogleAuthGuard } from './guards/google-auth.guard.js';
import { SessionAuthGuard } from './guards/session-auth.guard.js';
import { CurrentUser } from './decorators/current-user.decorator.js';
import { User } from '../users/entities/user.entity.js';
import { ConfigService } from '@nestjs/config';

@Controller('auth')
export class AuthController {
  constructor(
    private readonly authService: AuthService,
    private readonly configService: ConfigService,
  ) {}

  @Get('google')
  @UseGuards(GoogleAuthGuard)
  async googleLogin() {
    console.log('[AUTH DIAGNOSTIC] Initiating Google OAuth Login redirect...');
  }

  @Get('google/callback')
  @UseGuards(GoogleAuthGuard)
  async googleCallback(@Req() req: any, @Res() res: Response) {
    try {
      console.log('[OAUTH DIAGNOSTIC] Google callback received. req.user:', req.user);
      const user = await this.authService.validateUser(req.user);
      console.log('[OAUTH DIAGNOSTIC] Validated DB User:', user.email, 'ID:', user.id);

      req.session.userId = user.id;
      req.session.userEmail = user.email;

      const envFrontendUrl = (
        this.configService.get<string>('FRONTEND_URL', 'http://localhost:3000') ||
        'http://localhost:3000'
      ).replace(/\/$/, '');

      let frontendUrl = envFrontendUrl;
      const referer = req.headers.referer || req.headers.origin || '';
      if (referer.includes('dev-lms-v2.infinitelearningstudent.id')) {
        frontendUrl = 'https://dev-lms-v2.infinitelearningstudent.id';
      } else if (referer.includes('lms-v2.infinitelearningstudent.id')) {
        frontendUrl = 'https://lms-v2.infinitelearningstudent.id';
      }

      const token = req.sessionID;
      const dashboardUrl = `${frontendUrl}/dashboard?token=${token}`;
      console.log('[OAUTH DIAGNOSTIC] Session ID before save:', token);
      console.log('[OAUTH DIAGNOSTIC] Saving session & redirecting to:', dashboardUrl);

      return req.session.save((err: any) => {
        if (err) {
          console.error('[OAUTH DIAGNOSTIC ERROR] Session save error:', err);
        } else {
          console.log('[OAUTH DIAGNOSTIC SUCCESS] Session saved successfully for user:', user.email);
        }
        return res.redirect(dashboardUrl);
      });
    } catch (error: any) {
      console.error('[OAUTH DIAGNOSTIC ERROR] Callback failed:', error);
      const envFrontendUrl = (
        this.configService.get<string>('FRONTEND_URL', 'http://localhost:3000') ||
        'http://localhost:3000'
      ).replace(/\/$/, '');
      const errorMessage = encodeURIComponent(error.message || 'Login failed');
      return res.redirect(`${envFrontendUrl}/login?error=${errorMessage}`);
    }
  }

  @Get('me')
  @UseGuards(SessionAuthGuard)
  getCurrentUser(@CurrentUser() user: User) {
    console.log('[AUTH DIAGNOSTIC /auth/me SUCCESS] Returned profile for:', user.email);
    return user;
  }

  @Post('logout')
  @UseGuards(SessionAuthGuard)
  async logout(@Req() req: any, @Res() res: Response) {
    console.log('[AUTH DIAGNOSTIC] Logout requested for user:', req.session?.userEmail);
    req.session.destroy((err: any) => {
      if (err) {
        return res.status(500).json({ message: 'Failed to log out' });
      }
      res.clearCookie('connect.sid');
      return res.json({ message: 'Logged out' });
    });
  }
}
