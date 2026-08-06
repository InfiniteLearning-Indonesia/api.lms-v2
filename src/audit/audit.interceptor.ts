import {
  Injectable,
  NestInterceptor,
  ExecutionContext,
  CallHandler,
  HttpException,
} from '@nestjs/common';
import { Observable, throwError } from 'rxjs';
import { catchError, tap } from 'rxjs/operators';
import { AuditService } from './audit.service.js';

@Injectable()
export class AuditInterceptor implements NestInterceptor {
  constructor(private readonly auditService: AuditService) {}

  intercept(context: ExecutionContext, next: CallHandler): Observable<any> {
    const req = context.switchToHttp().getRequest();
    const res = context.switchToHttp().getResponse();
    const { method, url } = req;

    // Extract real client IP behind Nginx / Cloudflare reverse proxy
    const xForwardedFor = req.headers['x-forwarded-for'] as string;
    const xRealIp = req.headers['x-real-ip'] as string;
    const cfConnectingIp = req.headers['cf-connecting-ip'] as string;

    const clientIp =
      cfConnectingIp?.trim() ||
      xForwardedFor?.split(',')[0]?.trim() ||
      xRealIp?.trim() ||
      req.ip ||
      req.socket?.remoteAddress ||
      '127.0.0.1';

    // Skip GET requests to save DB resources, unless it's a critical endpoint
    if (method === 'GET') {
      return next.handle();
    }

    const user = req.user;
    const userId = user?.id;
    const userEmail = user?.email;
    const userRole = user?.roles ? user.roles.join(',') : user?.role;

    return next.handle().pipe(
      tap(() => {
        // Successful mutation
        this.auditService.logEvent({
          level: 'INFO',
          category: 'MUTATION',
          userId,
          userEmail,
          userRole,
          ipAddress: clientIp,
          action: `SUCCESS_${method}`,
          method,
          path: url,
          statusCode: res.statusCode,
          details: { query: req.query, params: req.params },
        });
      }),
      catchError((error) => {
        let level: 'WARN' | 'ERROR' = 'ERROR';
        let category: 'SECURITY' | 'AUTH' | 'SYSTEM' = 'SYSTEM';
        let status = 500;

        if (error instanceof HttpException) {
          status = error.getStatus();
          if (status === 401 || status === 403 || status === 429) {
            level = 'WARN';
            category = status === 401 ? 'AUTH' : 'SECURITY';
          }
        }

        this.auditService.logEvent({
          level,
          category,
          userId,
          userEmail,
          userRole,
          ipAddress: clientIp,
          action: `FAILED_${method}`,
          method,
          path: url,
          statusCode: status,
          details: {
            message: error.message,
            query: req.query,
            params: req.params,
          },
        });

        return throwError(() => error);
      }),
    );
  }
}
