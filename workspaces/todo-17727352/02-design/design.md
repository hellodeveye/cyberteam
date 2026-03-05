```json
{
  "design": {
    "system_overview": "基于微服务架构的分布式业务系统，采用领域驱动设计（DDD）思想，实现高可用、可扩展的业务能力。系统核心关注业务逻辑解耦、数据一致性保障和性能优化。",
    
    "module_division": {
      "user_service": {
        "responsibility": "用户身份管理、认证授权、个人信息维护",
        "core_functions": ["用户注册/登录", "权限验证", "用户信息管理", "会话管理"],
        "dependencies": ["auth_service", "notification_service"]
      },
      "auth_service": {
        "responsibility": "统一认证授权中心，负责令牌颁发、验证和权限校验",
        "core_functions": ["JWT令牌管理", "OAuth2.0集成", "权限策略管理", "访问控制"],
        "dependencies": ["user_service", "redis"]
      },
      "order_service": {
        "responsibility": "订单全生命周期管理，包括创建、支付、状态流转",
        "core_functions": ["订单创建", "支付处理", "状态机管理", "订单查询"],
        "dependencies": ["product_service", "payment_service", "inventory_service"]
      },
      "product_service": {
        "responsibility": "商品信息管理、库存查询、价格策略",
        "core_functions": ["商品CRUD", "库存管理", "价格计算", "分类管理"],
        "dependencies": ["inventory_service", "search_service"]
      },
      "payment_service": {
        "responsibility": "支付网关集成、交易处理、对账管理",
        "core_functions": ["支付渠道集成", "交易处理", "退款管理", "对账服务"],
        "dependencies": ["order_service", "external_payment_gateways"]
      },
      "notification_service": {
        "responsibility": "多渠道消息通知，支持实时和异步通知",
        "core_functions": ["消息模板管理", "多渠道发送（短信/邮件/推送）", "消息队列处理", "发送记录"],
        "dependencies": ["message_queue", "external_sms_email_services"]
      },
      "search_service": {
        "responsibility": "全文搜索和商品推荐服务",
        "core_functions": ["索引构建", "搜索查询", "相关性排序", "推荐算法"],
        "dependencies": ["elasticsearch", "product_service"]
      },
      "inventory_service": {
        "responsibility": "库存管理和分布式锁控制",
        "core_functions": ["库存扣减", "库存锁定", "库存同步", "库存预警"],
        "dependencies": ["redis_distributed_lock", "product_service"]
      }
    },
    
    "interface_definitions": {
      "user_service_interfaces": {
        "UserService": {
          "Register": {
            "request": "username, password, email, phone",
            "response": "user_id, token",
            "error_codes": ["USER_EXISTS", "INVALID_PARAMS"]
          },
          "Login": {
            "request": "username/email/phone, password",
            "response": "user_info, token, refresh_token",
            "error_codes": ["USER_NOT_FOUND", "PASSWORD_ERROR"]
          },
          "GetUserInfo": {
            "request": "user_id",
            "response": "user_profile, permissions, status",
            "error_codes": ["USER_NOT_FOUND", "UNAUTHORIZED"]
          }
        }
      },
      
      "order_service_interfaces": {
        "OrderService": {
          "CreateOrder": {
            "request": "user_id, items[{product_id, quantity}], shipping_address",
            "response": "order_id, total_amount, payment_url",
            "error_codes": ["INSUFFICIENT_STOCK", "PRODUCT_NOT_FOUND"]
          },
          "GetOrder": {
            "request": "order_id, user_id",
            "response": "order_details, status_history, payment_info",
            "error_codes": ["ORDER_NOT_FOUND", "UNAUTHORIZED"]
          },
          "CancelOrder": {
            "request": "order_id, user_id, reason",
            "response": "cancellation_id, refund_amount",
            "error_codes": ["ORDER_CANNOT_CANCEL", "INVALID_STATUS"]
          }
        }
      },
      
      "payment_service_interfaces": {
        "PaymentService": {
          "CreatePayment": {
            "request": "order_id, amount, payment_method, return_url",
            "response": "payment_id, payment_url, expires_at",
            "error_codes": ["PAYMENT_CREATE_FAILED", "INVALID_AMOUNT"]
          },
          "ProcessCallback": {
            "request": "payment_id, status, signature",
            "response": "processed, order_status",
            "error_codes": ["SIGNATURE_INVALID", "CALLBACK_EXPIRED"]
          }
        }
      }
    },
    
    "data_models": {
      "user_domain": {
        "User": {
          "fields": "id, username, email, phone, password_hash, salt, status, created_at, updated_at",
          "indexes": ["username_unique", "email_unique", "phone_unique"],
          "relations": "UserProfile(1:1), UserRoles(1:N)"
        },
        "UserProfile": {
          "fields": "user_id, real_name, avatar, gender, birthday, address",
          "indexes": ["user_id_primary"]
        },
        "UserRole": {
          "fields": "user_id, role_id, assigned_at, expires_at",
          "indexes": ["user_role_unique"]
        }
      },
      
      "order_domain": {
        "Order": {
          "fields": "id, order_no, user_id, total_amount, status, shipping_address, created_at, paid_at, completed_at",
          "indexes": ["order_no_unique", "user_id_index", "status_index"],
          "relations": "OrderItems(1:N), OrderPayments(1:N)"
        },
        "OrderItem": {
          "fields": "order_id, product_id, sku, quantity, unit_price, subtotal, product_snapshot",
          "indexes": ["order_product_index"]
        },
        "OrderPayment": {
          "fields": "order_id, payment_id, amount, payment_method, status, transaction_no, paid_at",
          "indexes": ["order_payment_index", "transaction_no_unique"]
        }
      },
      
      "product_domain": {
        "Product": {
          "fields": "id, sku, name, description, category_id, price, stock, status, attributes, created_at",
          "indexes": ["sku_unique", "category_index", "status_index"],
          "relations": "ProductCategory(N:1), ProductImages(1:N)"
        },
        "ProductCategory": {
          "fields": "id, name, parent_id, level, sort_order, is_leaf",
          "indexes": ["parent_id_index", "level_index"]
        }
      }
    },
    
    "database_design": {
      "postgresql_schemas": {
        "user_schema": "存储用户核心数据，ACID事务保障",
        "order_schema": "存储订单交易数据，强一致性要求",
        "product_schema": "存储商品目录数据，读多写少"
      },
      "redis_usage": {
        "session_cache": "用户会话和令牌缓存，TTL设置",
        "inventory_cache": "库存热点数据缓存，减少DB压力",
        "distributed_lock": "基于Redis的分布式锁，防止超卖",
        "rate_limiting": "API限流和防刷策略"
      }
    }
  },
  
  "architecture": "系统采用分层微服务架构，整体分为四层：\n\n1. 接入层（API Gateway）\n   - Kong/Nginx作为API网关，负责路由转发、负载均衡、SSL终止\n   - 统一认证拦截，JWT令牌验证\n   - 限流熔断，防止服务雪崩\n\n2. 业务服务层（Microservices）\n   - 8个独立微服务，每个服务独立部署、独立数据库\n   - 服务间通过gRPC进行通信，保证高性能和强类型\n   - 异步通信通过RabbitMQ/Kafka实现事件驱动\n   - 服务注册发现使用Consul/Etcd\n\n3. 数据存储层（Data Storage）\n   - PostgreSQL作为主数据库，分库分表设计\n   - Redis作为缓存层，支持会话、热点数据、分布式锁\n   - Elasticsearch提供全文搜索能力\n   - 对象存储（MinIO/S3）用于文件存储\n\n4. 基础设施层（Infrastructure）\n   - Docker容器化部署，Kubernetes编排管理\n   - Prometheus + Grafana监控告警\n   - ELK日志收集分析\n   - CI/CD流水线自动化部署\n\n关键架构模式：\n- 事件溯源（Event Sourcing）用于订单状态追踪\n- CQRS模式分离读写操作，优化查询性能\n- 断路器模式防止级联故障\n- 最终一致性通过消息队列保证",
  
  "tech_stack": ["Go", "PostgreSQL", "Redis", "gRPC", "RabbitMQ", "Elasticsearch", "Docker", "Kubernetes", "Consul", "Prometheus"],
  
  "tech_selection_reasons": {
    "Go": "1. 高性能并发处理，goroutine轻量级线程模型适合微服务\n2. 静态编译，部署简单，单个二进制文件包含所有依赖\n3. 丰富的标准库和成熟的微服务生态（gin, gorm, grpc）\n4. 优秀的工具链支持（go mod, go test, go vet）",
    
    "PostgreSQL": "1. ACID事务保证，适合订单、支付等强一致性场景\n2. JSONB支持半结构化数据存储\n3. 丰富的索引类型（GIN, GiST）和高级特性\n4. 成熟的复制和高可用方案",
    
    "Redis": "1. 内存级读写性能，适合缓存和会话管理\n2. 丰富的数据结构支持复杂业务场景\n3. 分布式锁和原子操作支持\n4. 持久化机制保证数据安全",
    
    "gRPC": "1. 基于HTTP/2的高性能RPC框架\n2. Protocol Buffers强类型接口定义\n3. 双向流、流控等高级特性\n4. 多语言支持，便于未来技术栈扩展",
    
    "RabbitMQ": "1. 成熟稳定的消息队列，社区活跃\n2. 多种Exchange类型支持复杂路由\n3. 消息持久化和ACK机制保证可靠性\n4. 管理界面完善，便于运维",
    
    "Docker_Kubernetes": "1. 容器化保证环境一致性\n2. Kubernetes提供自动扩缩容、服务发现、负载均衡\n3. 声明式配置，基础设施即代码\n4. 成熟的云原生生态"
  }
}
```

现在让我创建项目的基本结构来验证这个设计：

```bash
# 创建项目根目录
mkdir -p microservices-system
cd microservices-system

# 创建各服务目录
mkdir -p user-service/cmd internal/service internal/model internal/repository
mkdir -p auth-service/cmd internal/service internal/model
mkdir -p order-service/cmd internal/service internal/model
mkdir -p product-service/cmd internal/service internal/model
mkdir -p payment-service/cmd internal/service internal/model
mkdir -p notification-service/cmd internal/service internal/model
mkdir -p search-service/cmd internal/service internal/model
mkdir -p inventory-service/cmd internal/service internal/model

# 创建共享库目录
mkdir -p pkg/utils pkg/constants pkg/errors pkg/middleware

# 创建配置文件目录
mkdir -p configs deployments/docker deployments/kubernetes

# 创建文档目录
mkdir -p docs/api docs/architecture

# 初始化用户服务的Go模块
cd user-service
go mod init github.com/company/user-service

# 创建基础文件结构
cat > cmd/main.go << 'EOF'
package main

import (
	"log"
	
	"github.com/company/user-service/internal/server"
)

func main() {
	srv := server.NewServer()
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}
EOF

# 创建服务配置文件
cat > configs/config.yaml << 'EOF'
server:
  port: 8080
  mode: "debug"
  
database:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "password"
  dbname: "user_db"
  sslmode: "disable"
  
redis:
  addr: "localhost:6379"
  password: ""
  db: 0
  
jwt:
  secret: "your-secret-key"
  expires_in: 3600
EOF

# 创建用户模型
mkdir -p internal/model
cat > internal/model/user.go << 'EOF'
package model

import (
	"time"
	
	"gorm.io/gorm"
)

type User struct {
	ID           uint      `gorm:"primaryKey"`
	Username     string    `gorm:"uniqueIndex;size:50;not null"`
	Email        string    `gorm:"uniqueIndex;size:100;not null"`
	Phone        string    `gorm:"uniqueIndex;size:20"`
	PasswordHash string    `gorm:"size:255;not null"`
	Salt         string    `gorm:"size:50;not null"`
	Status       int       `gorm:"default:1;not null"` // 1: active, 0: inactive
	CreatedAt