package com.github.seungyeoplee.directorywatcher.tests.gracefulshutdownspring;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class GracefulShutdownSpringApplication {

    public static void main(String[] args) {
        SpringApplication.run(GracefulShutdownSpringApplication.class, args);
    }

}
