package main

import (
	"flag"
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

var withClient *bool
var withServer *bool
var withOTEL *bool

// main is the entry point for the application.
func main() {
	var flags flag.FlagSet
	withServer = flags.Bool("server", true, "enable server generation, default is true")
	withClient = flags.Bool("client", false, "enable client generation, default is false")
	withOTEL = flags.Bool("OpenTelemetry", false, "enable OpenTelemetry generation, default is false")
	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			generate(gen, f)
		}
		return nil
	})
}

// generate takes plugin and file as input, then output the generated file
func generate(plugin *protogen.Plugin, file *protogen.File) *protogen.GeneratedFile {
	filename := file.GeneratedFilenamePrefix + "_endpoints.pb.go"
	g := plugin.NewGeneratedFile(filename, file.GoImportPath)
	// 写入一些警告之类的 告诉用户不要修改
	g.P("// Code generated by protoc-gen-gokit-endpoint. DO NOT EDIT.")
	g.P("// versions:")
	g.P(fmt.Sprintf("// protoc-gen-gokit-endpoint %s", version))
	g.P()
	g.P("package ", file.GoPackageName)
	g.P()

	writeImport(g)

	for _, service := range file.Services {
		if *withServer {
			generateForService(g, service)
			generateServer(g, service)
		}
		if *withClient {
			generateClient(g, service)
		}
	}
	return g
}

func writeImport(file *protogen.GeneratedFile) {
	file.P("import (")
	file.P("\tcontext \"context\"")
	file.P("\tendpoint \"github.com/go-kit/kit/endpoint\"")
	file.P("\tgrpctransport \"github.com/go-kit/kit/transport/grpc\"")
	file.P("\tlog \"github.com/go-kit/kit/log\"")
	file.P("\tstdopentracing \"github.com/opentracing/opentracing-go\"\n")
	file.P("\topentracing \"github.com/go-kit/kit/tracing/opentracing\"\n")
	file.P("\ttracing \"codeup.aliyun.com/637b8bf70f175f12fbe05cc1/mock-demo/observability/tracing\"")
	if *withOTEL {
		file.P("\ttrace \"go.opentelemetry.io/otel/trace\"")
	}
	if *withClient {
		file.P("\tgrpc \"google.golang.org/grpc\"")
	}
	file.P(")")
}

func generateForService(file *protogen.GeneratedFile, service *protogen.Service) {
	for _, method := range service.Methods {
		if method.Desc.IsStreamingServer() || method.Desc.IsStreamingClient() {
			continue
		}
		generateGokitEndpoint(file, service, method)
		generateEncoderDecoder(file, service, method)
		generateHandler(file, service, method)
	}
}

func generateGokitEndpoint(file *protogen.GeneratedFile, service *protogen.Service, method *protogen.Method) {
	file.P(fmt.Sprintf("func make%sEndpoint(s %sServer) endpoint.Endpoint {", method.GoName, service.GoName))
	file.P("\treturn func(ctx context.Context, request interface{}) (interface{}, error) {")
	file.P(fmt.Sprintf("\t\treq := request.(*%s)", method.Input.GoIdent.GoName))
	file.P(fmt.Sprintf("\t\treturn s.%s(ctx, req)", method.GoName))
	file.P("\t}")
	file.P("}")
}

func generateEncoderDecoder(file *protogen.GeneratedFile, service *protogen.Service, method *protogen.Method) {
	file.P(fmt.Sprintf("func decode%sRequest(_ context.Context, req interface{}) (interface{}, error) {", method.GoName))
	file.P("\treturn req, nil")
	file.P("}")

	file.P(fmt.Sprintf("func encode%sResponse(_ context.Context, rsp interface{}) (interface{}, error) {", method.GoName))
	file.P("\treturn rsp, nil")
	file.P("}")
}

func generateHandler(file *protogen.GeneratedFile, service *protogen.Service, method *protogen.Method) {
	file.P(fmt.Sprintf("func Make%sHandler(s %sServer) *grpctransport.Server {", method.GoName, service.GoName))
	file.P(fmt.Sprintf("\treturn grpctransport.NewServer(make%sEndpoint(s), decode%sRequest, encode%sResponse )", method.GoName, method.GoName, method.GoName))
	file.P("}")
}

func generateServer(file *protogen.GeneratedFile, service *protogen.Service) {
	file.P(fmt.Sprintf("type %s struct {", service.GoName))
	file.P(fmt.Sprintf("\tUnimplemented%sServer", service.GoName))
	file.P(fmt.Sprintf("\tservice %sServer", service.GoName))
	file.P("\toptions []func(string) grpctransport.ServerOption")
	file.P("\tmiddlewares []func(endpoint.Endpoint) endpoint.Endpoint")
	file.P("\ttracer stdopentracing.Tracer")
	file.P("\ttp trace.TracerProvider")
	file.P("\tlogger log.Logger")
	for _, method := range service.Methods {
		if method.Desc.IsStreamingServer() || method.Desc.IsStreamingClient() {
			continue
		}
		file.P(fmt.Sprintf("\t\t%sHandler grpctransport.Handler", method.GoName))
	}
	file.P("}")

	file.P(fmt.Sprintf("func New%s(s %sServer, logger log.Logger) *%s {", service.GoName, service.GoName, service.GoName))
	file.P(fmt.Sprintf("\treturn &%s{", service.GoName))
	file.P("\t\tservice: s,")
	file.P("\t\tlogger: logger,")
	file.P("\t}")
	file.P("}")

	generateWithOptions(file, service)
	generateWithMiddlewares(file, service)
	generateWithTracing(file, service)
	generateServerWithTracerProvider(file, service)
	generateBuild(file, service)
	generateRegisterService(file, service)

	for _, method := range service.Methods {
		if method.Desc.IsStreamingServer() || method.Desc.IsStreamingClient() {
			continue
		}
		generateBuildMethod(file, service, method)
	}

	for _, method := range service.Methods {
		if method.Desc.IsStreamingServer() || method.Desc.IsStreamingClient() {
			continue
		}
		file.P(fmt.Sprintf("func (s *%s) %s(ctx context.Context, req *%s) (*%s, error) {", service.GoName, method.GoName, method.Input.GoIdent.GoName, method.Output.GoIdent.GoName))
		file.P(fmt.Sprintf("\t_, response, err := s.%sHandler.ServeGRPC(ctx, req)", method.GoName))
		file.P("\tif err != nil {")
		file.P("\t\treturn nil, err")
		file.P("\t}")
		file.P(fmt.Sprintf("\treturn response.(*%s), nil", method.Output.GoIdent.GoName))
		file.P("}")
		// generateGokitEndpoint(file, service, method)
		// generateEncoderDecoder(file, service, method)
		// generateHandler(file, service, method)
	}
}

func generateBuild(file *protogen.GeneratedFile, service *protogen.Service) {
	// generate Build  method
	file.P(fmt.Sprintf("func (s *%s) Build() {", service.GoName))
	for _, method := range service.Methods {
		if method.Desc.IsStreamingServer() || method.Desc.IsStreamingClient() {
			continue
		}
		file.P(fmt.Sprintf("\ts.build%s()", method.GoName))
	}
	file.P("}")
}

func generateRegisterService(file *protogen.GeneratedFile, service *protogen.Service) {
	file.P(fmt.Sprintf("func (s *%s) RegisterService(server *grpc.Server) {", service.GoName))
	file.P(fmt.Sprintf("\tRegister%sServer(server, s)", service.GoName))
	file.P("}")
}

func generateBuildMethod(file *protogen.GeneratedFile, service *protogen.Service, method *protogen.Method) {
	file.P(fmt.Sprintf("func (s *%s) build%s() {", service.GoName, method.GoName))
	file.P(fmt.Sprintf("\t%sEndpoint := make%sEndpoint(s.service)", method.GoName, method.GoName))
	file.P("\tvar ops []grpctransport.ServerOption")
	file.P("\tif s.tracer != nil {")
	file.P(fmt.Sprintf("\t\t%sEndpoint = opentracing.TraceServer(s.tracer, \"%s\")(%sEndpoint)",
		method.GoName,
		method.Desc.FullName(),
		method.GoName))
	file.P(fmt.Sprintf("\t\tops = append(ops, grpctransport.ServerBefore(opentracing.GRPCToContext(s.tracer, \"%s\", s.logger)))", method.Desc.FullName()))
	file.P("}")
	file.P(fmt.Sprintf("\tfor _, middleware := range s.middlewares {"))
	file.P(fmt.Sprintf("\t\t%sEndpoint = middleware(%sEndpoint)", method.GoName, method.GoName))
	file.P("\t}")
	if *withOTEL {
		file.P(fmt.Sprintf("\tops = append(ops, grpctransport.ServerBefore(tracing.GRPCToContext(s.tp, \"%s\", s.logger)))", method.Desc.FullName()))
		file.P(fmt.Sprintf("\t%sEndpoint = tracing.EndpointMiddleware(\"%s\")(%sEndpoint)", method.GoName, method.Desc.FullName(), method.GoName))
	}
	file.P("\tfor _, option := range s.options {")
	file.P(fmt.Sprintf("\t\tops = append(ops, option(\"%s\"))", method.Desc.FullName()))
	file.P("\t}")
	file.P(fmt.Sprintf("\ts.%sHandler = grpctransport.NewServer(%sEndpoint, decode%sRequest, encode%sResponse, ops...)",
		method.GoName,
		method.GoName,
		method.GoName,
		method.GoName))
	file.P("}")
}

func generateWithOptions(file *protogen.GeneratedFile, service *protogen.Service) {
	// generate WithOptions method
	file.P(fmt.Sprintf("func (s *%s) WithOptions(options ...func(string) grpctransport.ServerOption) {", service.GoName))
	file.P("\ts.options = options")
	file.P("}")
}

func generateWithMiddlewares(file *protogen.GeneratedFile, service *protogen.Service) {
	// generate WithMiddlewares method
	file.P(fmt.Sprintf("func (s *%s) WithMiddlewares(middlewares ...func(endpoint.Endpoint) endpoint.Endpoint) {", service.GoName))
	file.P("\ts.middlewares = middlewares")
	file.P("}")
}

func generateWithTracing(file *protogen.GeneratedFile, service *protogen.Service) {
	file.P(fmt.Sprintf("func (s *%s) WithTracing(tracer stdopentracing.Tracer) {", service.GoName))
	file.P("\ts.tracer = tracer")
	file.P("}")
}

func generateServerWithTracerProvider(file *protogen.GeneratedFile, service *protogen.Service) {
	file.P(fmt.Sprintf("func (s *%s) WithTracerProvider(tp trace.TracerProvider) {", service.GoName))
	file.P("\ts.tp = tp")
	file.P("}")
}

//
// func generateController(file *protogen.GeneratedFile, service *protogen.Service) {
//    file.P(fmt.Sprintf("type %sController struct {", service.GoName))
//    file.P(fmt.Sprintf("\tserver %sServer", service.GoName))
//    // writing GetControllerType function
//    //file.P(fmt.Sprintf("    GetControllerType() reflect.Type"))
//    // walk through all the service method and generate function
//    for _, method := range service.Methods {
//        if method.Desc.IsStreamingServer() || method.Desc.IsStreamingClient() {
//            continue
//        }
//        file.P(fmt.Sprintf("    %s func(ctx *http.Context) `%s`",
//            method.GoName,
//            getMethodTag(method)))
//    }
//    file.P("}\n\n")
//    generateConstructor(file, service)
//    file.P("\n\n")
//    generateInterfaceImplement(file, service)
//    if *enableRpc {
//        generateRpcClient(file, service)
//    }
// }
//
// // generateMethodTags  will accept the method and generate the tags for the method
// func getMethodTag(method *protogen.Method) string {
//    value := proto.GetExtension(method.Desc.Options(), annotations.E_Http)
//    rule := value.(*annotations.HttpRule)
//    if rule != nil {
//        path, method := resolveHttpMethod(rule)
//        // TODO: 解析参数
//        return fmt.Sprintf("method:\"%s\" route:\"%s\" param:\"input\"", method, path)
//    }
//    return fmt.Sprintf("method:\"GET\"")
// }
//
// func resolveHttpMethod(rule *annotations.HttpRule) (string, string) {
//    var path string
//    var method string
//    switch pattern := rule.Pattern.(type) {
//    case *annotations.HttpRule_Get:
//        path = pattern.Get
//        method = "GET"
//    case *annotations.HttpRule_Put:
//        path = pattern.Put
//        method = "PUT"
//    case *annotations.HttpRule_Post:
//        path = pattern.Post
//        method = "POST"
//    case *annotations.HttpRule_Delete:
//        path = pattern.Delete
//        method = "DELETE"
//    case *annotations.HttpRule_Patch:
//        path = pattern.Patch
//        method = "PATCH"
//    case *annotations.HttpRule_Custom:
//        path = pattern.Custom.Path
//        method = pattern.Custom.Kind
//    }
//    return path, method
// }
//
// func generateConstructor(file *protogen.GeneratedFile, service *protogen.Service) {
//    file.P(fmt.Sprintf("func New%sController(server %sServer) *%sController {",
//        service.GoName,
//        service.GoName,
//        service.GoName))
//    file.P(fmt.Sprintf("    return &%sController{server: server}", service.GoName))
//    file.P("}")
// }
//
// func generateInterfaceImplement(file *protogen.GeneratedFile, service *protogen.Service) {
//    file.P(fmt.Sprintf("func (c *%sController) GetControllerType() reflect.Type {", service.GoName))
//    file.P(fmt.Sprintf("    return reflect.TypeOf(*c)"))
//    file.P("}")
// }
