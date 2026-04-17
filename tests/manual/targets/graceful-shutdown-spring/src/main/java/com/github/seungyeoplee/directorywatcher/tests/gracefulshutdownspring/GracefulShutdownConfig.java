package com.github.seungyeoplee.directorywatcher.tests.gracefulshutdownspring;

import org.springframework.context.annotation.Configuration;
import org.springframework.context.event.EventListener;
import org.springframework.context.event.ContextClosedEvent;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.time.LocalDateTime;


@Configuration
public class GracefulShutdownConfig {
    
    @EventListener(ContextClosedEvent.class)
    public void onContextClosed() throws InterruptedException {
        // 애플리케이션 종료 시 1초간 대기하는 작업 실행
        Thread.sleep(1000);
        try {
            Files.write(
                    Paths.get("logs/shutdown_log_" + LocalDateTime.now().format(java.time.format.DateTimeFormatter.ofPattern("yyyyMMdd_HHmmss")) + ".txt"),
                    "Application shutdown completed after 1 seconds delay".getBytes()
            );
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
    }
}
