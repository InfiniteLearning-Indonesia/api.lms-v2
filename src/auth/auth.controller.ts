import {
  Body,
  Controller,
  Get,
  Post,
  Req,
  Res,
  UseGuards,
  UnauthorizedException,
  Logger,
} from '@nestjs/common';
import type { Response } from 'express';
import { AuthService } from './auth.service.js';
import { GoogleAuthGuard } from './guards/google-auth.guard.js';
import { SessionAuthGuard } from './guards/session-auth.guard.js';
import { CurrentUser } from './decorators/current-user.decorator.js';
import { User } from '../users/entities/user.entity.js';
import { ConfigService } from '@nestjs/config';
import { Throttle } from '@nestjs/throttler';
import * as crypto from 'crypto';

@Controller('auth')
export class AuthController {
  private static exchangeCodes = new Map<
    string,
    { sessionID: string; userId: string; expiresAt: number }
  >();

  private static cleanupExpiredCodes(): void {
    const now = Date.now();
    for (const [key, entry] of AuthController.exchangeCodes.entries()) {
      if (entry.expiresAt < now) {
        AuthController.exchangeCodes.delete(key);
      }
    }
  }
  private readonly logger = new Logger(AuthController.name);

  constructor(
    private readonly authService: AuthService,
    private readonly configService: ConfigService,
  ) {}

  @Get('google')
  @UseGuards(GoogleAuthGuard)
  async googleLogin() {}

  @Get('google/callback')
  @UseGuards(GoogleAuthGuard)
  async googleCallback(@Req() req: any, @Res() res: Response) {
    try {
      const user = await this.authService.validateUser(req.user);

      req.session.userId = user.id;
      req.session.userEmail = user.email;

      const envFrontendUrl = (
        this.configService.get<string>(
          'FRONTEND_URL',
          'http://localhost:3000',
        ) || 'http://localhost:3000'
      ).replace(/\/$/, '');

      let frontendUrl = envFrontendUrl;
      const referer = req.headers.referer || req.headers.origin || '';
      if (referer.includes('dev-lms-v2.infinitelearningstudent.id')) {
        frontendUrl = 'https://dev-lms-v2.infinitelearningstudent.id';
      } else if (referer.includes('lms-v2.infinitelearningstudent.id')) {
        frontendUrl = 'https://lms-v2.infinitelearningstudent.id';
      }

      // Generate a short-lived exchange code that maps to the session
      AuthController.cleanupExpiredCodes();
      const exchangeCode = crypto.randomBytes(32).toString('hex');
      AuthController.exchangeCodes.set(exchangeCode, {
        sessionID: req.sessionID,
        userId: user.id,
        expiresAt: Date.now() + 30000, // 30 seconds
      });
      const dashboardUrl = `${frontendUrl}/dashboard?code=${exchangeCode}`;

      return req.session.save((err: any) => {
        if (err) {
          this.logger.error('Session save error:', err);
        }
        return res.redirect(dashboardUrl);
      });
    } catch (error: any) {
      this.logger.error('Callback failed:', error);
      const envFrontendUrl = (
        this.configService.get<string>(
          'FRONTEND_URL',
          'http://localhost:3000',
        ) || 'http://localhost:3000'
      ).replace(/\/$/, '');
      const errorMessage = encodeURIComponent(error.message || 'Login failed');
      return res.redirect(`${envFrontendUrl}/login?error=${errorMessage}`);
    }
  }

  @Post('exchange-code')
  async exchangeCode(@Body() body: { code: string }, @Req() req: any) {
    const entry = AuthController.exchangeCodes.get(body.code);
    if (!entry || entry.expiresAt < Date.now()) {
      throw new UnauthorizedException('Kode tidak valid atau kedaluwarsa.');
    }
    AuthController.exchangeCodes.delete(body.code);

    // Bind this session to the user
    req.session.userId = entry.userId;
    return new Promise((resolve, reject) => {
      req.session.save((err: any) => {
        if (err) return reject(err);
        resolve({ token: req.sessionID });
      });
    });
  }

  @Throttle({ default: { limit: 10, ttl: 60000 } })
  @Post('check-status')
  async checkStatus(@Body() body: { email: string }) {
    return this.authService.checkEmailStatus(body.email);
  }

  @Throttle({ default: { limit: 5, ttl: 60000 } })
  @Post('login-local')
  async loginLocal(
    @Body() body: { email: string; password?: string },
    @Req() req: any,
  ) {
    const user = await this.authService.validateLocalUser(
      body.email,
      body.password || '',
    );
    req.session.userId = user.id;
    req.session.userEmail = user.email;

    return new Promise((resolve, reject) => {
      req.session.save((err: any) => {
        if (err) return reject(err);
        resolve({
          message: 'Login berhasil',
          token: req.sessionID,
          user: {
            id: user.id,
            email: user.email,
            name: user.name,
            roles: user.roles,
          },
        });
      });
    });
  }

  @Throttle({ default: { limit: 5, ttl: 60000 } })
  @Post('setup-password')
  async setupPassword(
    @Body() body: { email: string; password?: string },
    @Req() req: any,
  ) {
    const user = await this.authService.setupInitialPassword(
      body.email,
      body.password || '',
    );
    req.session.userId = user.id;
    req.session.userEmail = user.email;

    return new Promise((resolve, reject) => {
      req.session.save((err: any) => {
        if (err) return reject(err);
        resolve({
          message: 'Password berhasil dibuat dan login berhasil',
          token: req.sessionID,
          user: {
            id: user.id,
            email: user.email,
            name: user.name,
            roles: user.roles,
          },
        });
      });
    });
  }

  @Post('change-password')
  @UseGuards(SessionAuthGuard)
  async changePassword(
    @CurrentUser() user: User,
    @Body() body: { currentPassword?: string; newPassword?: string },
  ) {
    const updatedUser = await this.authService.changePassword(
      user.id,
      body.currentPassword || '',
      body.newPassword || '',
    );
    return {
      message: 'Password berhasil diubah!',
      user: updatedUser,
    };
  }

  @Get('me')
  @UseGuards(SessionAuthGuard)
  getCurrentUser(@CurrentUser() user: User) {
    this.logger.log(`Returned profile for: ${user.email}`);
    return user;
  }

  @Post('logout')
  @UseGuards(SessionAuthGuard)
  async logout(@Req() req: any, @Res() res: Response) {
    this.logger.log(`Logout requested for user: ${req.session?.userEmail}`);

    const frontendUrl = (
      this.configService.get<string>('FRONTEND_URL', 'http://localhost:3000') ||
      'http://localhost:3000'
    ).replace(/\/$/, '');
    const isHttps =
      this.configService.get<string>('NODE_ENV') === 'production' ||
      frontendUrl.startsWith('https://');
    const isInfiniteDomain =
      frontendUrl.includes('infinitelearningstudent.id') ||
      this.configService.get<string>('NODE_ENV') === 'production';

    req.session.destroy((err: any) => {
      if (err) {
        return res.status(500).json({ message: 'Failed to log out' });
      }
      res.clearCookie('connect.sid', {
        path: '/',
        domain: isInfiniteDomain ? '.infinitelearningstudent.id' : undefined,
        secure: isHttps,
        sameSite: isHttps ? 'none' : 'lax',
      });
      return res.json({ message: 'Logged out' });
    });
  }
}
