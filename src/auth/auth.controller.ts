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
    // Handled automatically by Passport Google strategy
  }

  @Get('google/callback')
  @UseGuards(GoogleAuthGuard)
  async googleCallback(@Req() req: any, @Res() res: Response) {
    try {
      const user = await this.authService.validateUser(req.user);
      req.session.userId = user.id;
      req.session.userEmail = user.email;

      const frontendUrl = (this.configService.get<string>(
        'FRONTEND_URL',
        'http://localhost:3000',
      ) || 'http://localhost:3000').replace(/\/$/, '');

      const dashboardUrl = this.configService.get<string>(
        'FRONTEND_DASHBOARD_URL',
        `${frontendUrl}/dashboard`,
      );
      return res.redirect(dashboardUrl);
    } catch (error: any) {
      // Redirect to frontend login with error query param
      const frontendUrl = (this.configService.get<string>(
        'FRONTEND_URL',
        'http://localhost:3000',
      ) || 'http://localhost:3000').replace(/\/$/, '');
      const errorMessage = encodeURIComponent(error.message || 'Login failed');
      return res.redirect(`${frontendUrl}/login?error=${errorMessage}`);
    }
  }

  @Get('me')
  @UseGuards(SessionAuthGuard)
  getCurrentUser(@CurrentUser() user: User) {
    return user;
  }

  @Post('logout')
  @UseGuards(SessionAuthGuard)
  async logout(@Req() req: any, @Res() res: Response) {
    req.session.destroy((err: any) => {
      if (err) {
        return res.status(500).json({ message: 'Failed to log out' });
      }
      res.clearCookie('connect.sid'); // Clear session cookie
      return res.json({ message: 'Logged out' });
    });
  }
}
